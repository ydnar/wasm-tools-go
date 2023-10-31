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

/*
package wasi:clocks;

interface wall-clock {
    record datetime {
        seconds: u64,
        nanoseconds: u32
    };
    now: func() -> datetime;
    resolution: func() -> datetime;
}

world imports {
    import wall-clock;
}
*/
