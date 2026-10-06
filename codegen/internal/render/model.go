package render

import "github.com/relux-works/javacard-rpc/pluginapi"

type Schema = pluginapi.Schema
type Applet = pluginapi.Applet
type Method = pluginapi.Method
type Message = pluginapi.Message
type FieldType = pluginapi.FieldType
type ParameterLocation = pluginapi.ParameterLocation
type Field = pluginapi.Field
type StatusWord = pluginapi.StatusWord

const (
	FieldTypeU8           = pluginapi.FieldTypeU8
	FieldTypeU16          = pluginapi.FieldTypeU16
	FieldTypeU32          = pluginapi.FieldTypeU32
	FieldTypeBool         = pluginapi.FieldTypeBool
	FieldTypeASCII        = pluginapi.FieldTypeASCII
	FieldTypeString       = pluginapi.FieldTypeString
	FieldTypeBytes        = pluginapi.FieldTypeBytes
	FieldTypeBytesFixed   = pluginapi.FieldTypeBytesFixed
	FieldTypeStream       = pluginapi.FieldTypeStream
	ParameterLocationNone = pluginapi.ParameterLocationNone
	ParameterLocationP1   = pluginapi.ParameterLocationP1
	ParameterLocationP2   = pluginapi.ParameterLocationP2
	ParameterLocationData = pluginapi.ParameterLocationData
)
