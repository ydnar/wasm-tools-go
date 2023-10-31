package wallclock

type DateTime struct {
	Seconds      uint64
	Nanonseconds uint32
}

type WallClock interface {
	Now() DateTime
	Resolution() DateTime
}

var WallClockExport WallClock

//go:wasmexport wasi:clocks/wall-clock now
func wasmexport_now() DateTime {
	return WallClockExport.Now()
}

//go:wasmexport wasi:clocks/wall-clock resolution
func wasmexport_resolution() DateTime {
	return WallClockExport.Resolution()
}
