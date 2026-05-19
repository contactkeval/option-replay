package model

type ParsedTicker struct {
	Underlying string
	Expiry     string
	OptionType bool
	Strike     uint32
}
