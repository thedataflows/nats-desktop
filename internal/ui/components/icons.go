package components

import (
	"image/color"

	"gioui.org/layout"
	"github.com/thedataflows/nats-desktop/internal/icons"
)

type Icon interface {
	Layout(gtx layout.Context, color color.NRGBA) layout.Dimensions
}

var (
	CloseIcon       = icons.NavigationClose
	CleanIcon       = icons.ActionDelete
	InfoIcon        = icons.ActionInfo
	WarnIcon        = icons.AlertWarning
	ErrorIcon       = icons.AlertError
	WarningIcon     = icons.AlertWarning
	ExpandIcon      = icons.NavigationExpandMore
	ForwardIcon     = icons.NavigationChevronRight
	CircleIcon      = icons.ActionAccountCircle
	MoreVertIcon    = icons.NavigationMoreVert
	AddIcon         = icons.ContentAddCircle
	RefreshIcon     = icons.NavigationRefresh
	PauseIcon       = icons.AVPause
	WatchIcon       = icons.ActionVisibility
	UploadIcon      = icons.FileFileUpload
	SendIcon        = icons.ContentSend
	PlayIcon        = icons.AVPlayArrow
	CheckIcon       = icons.ActionCheckCircle
	DownloadIcon    = icons.FileFileDownload
	ConnectionsIcon = icons.ActionSwapHoriz
	StreamsIcon     = icons.DeviceStorage
	ConsumersIcon   = icons.ActionInput
	KVIcon          = icons.ActionLabel
	ObjectsIcon     = icons.FileCloudUpload
	ServicesIcon    = icons.ActionExtension
)

type IconPosition int

const (
	IconPositionStart IconPosition = iota
	IconPositionEnd
)
