// Copyright 2024 The NATS Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package tracing

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/klauspost/compress/s2"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
)

const (
	ErrTraceMsgMissing    = "This trace event was missing"
	unknownServerName     = "<Unknown>"
	contentEncodingHeader = "Content-Encoding"
)

func TraceMsg(nc *nats.Conn, traceMsg *nats.Msg, deliverToDest bool, timeout time.Duration) (*server.MsgTraceEvent, error) {
	sub, err := nc.SubscribeSync(nc.NewRespInbox())
	if err != nil {
		return nil, err
	}
	defer sub.Unsubscribe()

	err = nc.Flush()
	if err != nil {
		return nil, err
	}

	time.Sleep(250 * time.Millisecond)

	newMsg := nats.NewMsg(traceMsg.Subject)

	newMsg.Data = make([]byte, len(traceMsg.Data))
	copy(newMsg.Data, traceMsg.Data)

	for k, vs := range traceMsg.Header {
		for _, v := range vs {
			newMsg.Header.Add(k, v)
		}
	}
	newMsg.Header.Set("Accept-Encoding", "snappy")
	newMsg.Header.Set(server.MsgTraceDest, sub.Subject)
	if !deliverToDest {
		newMsg.Header.Set(server.MsgTraceOnly, "true")
	}

	err = nc.PublishMsg(newMsg)
	if err != nil {
		return nil, err
	}

	return GetMsgTrace(sub, sub.Subject, timeout)
}

func GetMsgTrace(sub *nats.Subscription, traceSubject string, timeout time.Duration) (*server.MsgTraceEvent, error) {
	var (
		origin  *server.MsgTraceEvent
		traces  = map[string]*server.MsgTraceEvent{}
		missing = map[string]*server.MsgTraceEvent{}
		servers = map[string]*server.MsgTraceEvent{}
	)

	var retErr error
	setErr := func(err error) {
		if retErr != nil {
			return
		}
		retErr = err
	}

	for {
		msg, err := sub.NextMsg(timeout)
		if err != nil {
			setErr(err)
			break
		}
		if msg.Subject != traceSubject {
			continue
		}
		data, err := getData(msg)
		if err != nil {
			setErr(err)
			continue
		}

		var e *server.MsgTraceEvent
		if err = json.Unmarshal(data, &e); err != nil {
			setErr(err)
			continue
		}
		ingress := e.Ingress()
		if ingress == nil {
			setErr(fmt.Errorf("missing ingress in trace event: %+v", e))
			continue
		}
		if ingress.Kind == server.CLIENT {
			if origin != nil {
				setErr(fmt.Errorf("duplicate ingress from client: %+v", ingress))
				continue
			}
			origin = e
		} else {
			hop := nats.Header(e.Request.Header).Get(server.MsgTraceHop)
			if hop == "" {
				setErr(fmt.Errorf("event for this remote should have a 'Hop' header, but it did not: %+v", e))
				continue
			}
			traces[hop] = e
		}
		servers[e.Server.Name] = e
		if gotAllServers(servers, origin != nil) {
			break
		}
	}

	sorted := make([]string, 0, len(traces))
	for k := range traces {
		sorted = append(sorted, k)
	}
	sort.Strings(sorted)
	for _, hop := range sorted {
		origin = ensureParentExists(hop, origin, traces, missing)
	}
	linkEgresses(origin, traces)

	return origin, retErr
}

func gotAllServers(all map[string]*server.MsgTraceEvent, hasOrigin bool) bool {
	if !hasOrigin {
		return false
	}
	for _, tr := range all {
		in := tr.Ingress()
		if in.Kind != server.CLIENT {
			if _, ok := all[in.Name]; !ok {
				return false
			}
		}
		if tr.Hops > 0 {
			egresses := tr.Egresses()
			for _, eg := range egresses {
				if eg.Kind == server.CLIENT {
					continue
				}
				if _, ok := all[eg.Name]; !ok {
					return false
				}
			}
		}
	}
	return true
}

func getData(m *nats.Msg) ([]byte, error) {
	data := m.Data
	eh := m.Header.Get(contentEncodingHeader)
	switch eh {
	case "gzip":
		zr, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		data, err = io.ReadAll(zr)
		if err != nil && err != io.ErrUnexpectedEOF {
			return nil, err
		}
		if err = zr.Close(); err != nil {
			return nil, err
		}
	case "snappy":
		var err error
		sr := s2.NewReader(bytes.NewReader(data))
		data, err = io.ReadAll(sr)
		if err != nil && err != io.ErrUnexpectedEOF {
			return nil, err
		}
	}
	return data, nil
}

func ensureParentExists(hop string, origin *server.MsgTraceEvent, traces, missing map[string]*server.MsgTraceEvent) *server.MsgTraceEvent {
	if hop == "" {
		return origin
	}

	var parentHop string
	var parentTrace *server.MsgTraceEvent

	child := traces[hop]
	ingress := child.Ingress()
	pos := strings.LastIndexByte(hop, '.')
	if pos != -1 {
		parentHop = hop[:pos]
		parentTrace = traces[parentHop]
	} else {
		parentTrace = origin
	}
	if parentTrace == nil {
		parentTrace = addMissingTraceEvent(ingress, hop, child, pos == -1)
		missing[parentHop] = parentTrace
		if pos == -1 {
			origin = parentTrace
		} else {
			traces[parentHop] = parentTrace
		}
	} else if _, parentWasMissing := missing[parentHop]; parentWasMissing {
		egresses := parentTrace.Egresses()
		found := false
		for _, eg := range egresses {
			if eg.Name == child.Server.Name {
				found = true
				break
			}
		}
		if !found {
			parentTrace.Events = append(parentTrace.Events, &server.MsgTraceEgress{
				MsgTraceBase: server.MsgTraceBase{Type: server.MsgTraceEgressType},
				Kind:         ingress.Kind,
				Name:         child.Server.Name,
				Hop:          hop,
			})
		}
	}
	return ensureParentExists(parentHop, origin, traces, missing)
}

func addMissingTraceEvent(ingress *server.MsgTraceIngress, hop string, child *server.MsgTraceEvent, parentIsOrigin bool) *server.MsgTraceEvent {
	ikind := ingress.Kind
	if parentIsOrigin {
		ikind = server.CLIENT
	}
	in := &server.MsgTraceIngress{
		MsgTraceBase: server.MsgTraceBase{Type: server.MsgTraceIngressType},
		Kind:         ikind,
		Error:        ErrTraceMsgMissing,
	}
	eg := &server.MsgTraceEgress{
		MsgTraceBase: server.MsgTraceBase{Type: server.MsgTraceEgressType},
		Kind:         ingress.Kind,
		Name:         child.Server.Name,
		Hop:          hop,
	}
	srvName := ingress.Name
	if srvName == "" {
		srvName = unknownServerName
	}
	return &server.MsgTraceEvent{
		Server: server.ServerInfo{Name: srvName},
		Events: []server.MsgTrace{in, eg},
	}
}

func linkEgresses(e *server.MsgTraceEvent, m map[string]*server.MsgTraceEvent) {
	if e == nil {
		return
	}
	ingress := e.Ingress()
	egresses := e.Egresses()
	for _, eg := range egresses {
		if eg.Kind == server.CLIENT {
			continue
		}
		k := eg.Hop
		if link, ok := m[k]; ok {
			delete(m, k)
			eg.Link = link
			if ingress.Error != ErrTraceMsgMissing {
				linkIngress := link.Ingress()
				if linkIngress.Error == ErrTraceMsgMissing {
					if eg.Name != link.Server.Name {
						link.Server.Name = eg.Name
					}
					if eg.Kind != linkIngress.Kind {
						linkIngress.Kind = eg.Kind
					}
				}
			}
			linkEgresses(link, m)
		}
	}
}

func ServerKindString(kind int) string {
	switch kind {
	case server.CLIENT:
		return "Client"
	case server.JETSTREAM:
		return "JetStream"
	case server.ROUTER:
		return "Router"
	case server.GATEWAY:
		return "Gateway"
	case server.LEAF:
		return "Leafnode"
	default:
		return "Unknown"
	}
}

func KindToArrow(kind int) string {
	switch kind {
	case server.CLIENT:
		return "--C"
	case server.JETSTREAM:
		return "--J"
	case server.ROUTER:
		return "-->"
	case server.GATEWAY:
		return "==>"
	case server.LEAF:
		return "~~>"
	default:
		return "---"
	}
}
