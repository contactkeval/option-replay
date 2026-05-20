package stage3_sort_dedupe

type Stage3Row struct {
	Strike      uint32
	OptionType  bool
	WindowStart uint32

	Open  uint32
	High  uint32
	Low   uint32
	Close uint32

	Volume       uint32
	Transactions uint32
}
