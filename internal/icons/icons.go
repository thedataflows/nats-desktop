package icons

import (
	"image/color"

	"gioui.org/layout"
	"gioui.org/widget"

	"golang.org/x/exp/shiny/materialdesign/icons"
)

type Icon interface {
	Layout(gtx layout.Context, color color.NRGBA) layout.Dimensions
}

func mustIcon(b []byte) *widget.Icon {
	icon, _ := widget.NewIcon(b)
	return icon
}

var (
	ActionSwapHoriz        = mustIcon(icons.ActionSwapHoriz)
	ActionSettingsEthernet = mustIcon(icons.ActionSettingsEthernet)
	DeviceStorage          = mustIcon(icons.DeviceStorage)
	ActionInput            = mustIcon(icons.ActionInput)
	ActionLabel            = mustIcon(icons.ActionLabel)
	FileCloudUpload        = mustIcon(icons.FileCloudUpload)
	ActionExtension        = mustIcon(icons.ActionExtension)
	ContentSend            = mustIcon(icons.ContentSend)
	EditorInsertChart      = mustIcon(icons.EditorInsertChart)
	ActionHistory          = mustIcon(icons.ActionHistory)
	FileCloudDownload      = mustIcon(icons.FileCloudDownload)
	ActionDescription      = mustIcon(icons.ActionDescription)
	ActionAccountCircle    = mustIcon(icons.ActionAccountCircle)
	ActionSettings         = mustIcon(icons.ActionSettings)
	ContentAddCircle       = mustIcon(icons.ContentAddCircle)
	NavigationRefresh      = mustIcon(icons.NavigationRefresh)
	ActionDelete           = mustIcon(icons.ActionDelete)
	ActionSearch           = mustIcon(icons.ActionSearch)
	NavigationClose        = mustIcon(icons.NavigationClose)
	ContentDeleteSweep     = mustIcon(icons.ContentDeleteSweep)
	ActionInfo             = mustIcon(icons.ActionInfo)
	ActionVisibility       = mustIcon(icons.ActionVisibility)
	ActionVisibilityOff    = mustIcon(icons.ActionVisibilityOff)
	AVPlayArrow            = mustIcon(icons.AVPlayArrow)
	AVPause                = mustIcon(icons.AVPause)
	ContentContentCopy     = mustIcon(icons.ContentContentCopy)
	EditorModeEdit         = mustIcon(icons.EditorModeEdit)
	ActionCheckCircle      = mustIcon(icons.ActionCheckCircle)
	AlertWarning           = mustIcon(icons.AlertWarning)
	AlertError             = mustIcon(icons.AlertError)
	FileFileDownload       = mustIcon(icons.FileFileDownload)
	FileFileUpload         = mustIcon(icons.FileFileUpload)
	NavigationMenu         = mustIcon(icons.NavigationMenu)
	NavigationArrowBack    = mustIcon(icons.NavigationArrowBack)
	ContentFilterList      = mustIcon(icons.ContentFilterList)
	ContentSort            = mustIcon(icons.ContentSort)
	NavigationChevronRight = mustIcon(icons.NavigationChevronRight)
	NavigationExpandMore   = mustIcon(icons.NavigationExpandMore)
	NavigationUnfoldMore   = mustIcon(icons.NavigationUnfoldMore)
	ImageBrightness2       = mustIcon(icons.ImageBrightness2)
	SocialNotifications    = mustIcon(icons.SocialNotifications)
	HardwareDesktopMac     = mustIcon(icons.HardwareDesktopMac)
	NavigationMoreVert     = mustIcon(icons.NavigationMoreVert)
	NavigationMoreHoriz    = mustIcon(icons.NavigationMoreHoriz)
	HardwareKeyboard       = mustIcon(icons.HardwareKeyboard)
	ActionHelp             = mustIcon(icons.ActionHelp)
)
