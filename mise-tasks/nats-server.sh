#!/usr/bin/env bash

#MISE description="Start local NATS server with JetStream and system account"
#MISE alias="s"

mkdir ./tmp
cat > ./tmp/nats-server.conf <<!
jetstream {
    store_dir = "./tmp"
}

system_account: SYS

accounts {
    SYS {
        users: [{user: admin, password: admin}]
    }
    default {
        users: [{user: user, password: user}]
        jetstream: enabled
    }
}
!

nats-server -c ./tmp/nats-server.conf
