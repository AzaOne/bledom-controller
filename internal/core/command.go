package core

// CommandType defines the type of command being dispatched.
type CommandType string

const (
	CmdSetPower           CommandType = "setPower"
	CmdSetColor           CommandType = "setColor"
	CmdSetBrightness      CommandType = "setBrightness"
	CmdSetSpeed           CommandType = "setSpeed"
	CmdSetHardwarePattern CommandType = "setHardwarePattern"
	CmdSyncTime           CommandType = "syncTime"
	CmdSetRgbOrder        CommandType = "setRgbOrder"
	CmdSetSchedule        CommandType = "setSchedule"
	CmdRunPattern         CommandType = "runPattern"
	CmdStopPattern        CommandType = "stopPattern"
	CmdAddSchedule        CommandType = "addSchedule"
	CmdRemoveSchedule     CommandType = "removeSchedule"
	CmdGetPatternCode     CommandType = "getPatternCode"
	CmdSavePatternCode    CommandType = "savePatternCode"
	CmdDeletePattern      CommandType = "deletePattern"
)

// Command is the envelope for incoming requests to change state or perform actions.
type Command struct {
	Type    CommandType
	Payload map[string]interface{}
}

// CommandChannel is the single channel that the core Agent listens to for commands.
type CommandChannel chan Command
