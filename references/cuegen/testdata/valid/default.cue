package test

Default1: {
	a1: *"abc" | string
	// empty string
	a2: *"" | string
	b1: *true | bool
	b2: *false | bool
	c1: *123 | int
	c2: *123 | int8
	c3: *123 | int16
	c4: *123 | int32
	c5: *123 | int64
	d1: *123 | uint
	d2: *123 | uint8
	d3: *123 | uint16
	d4: *123 | uint32
	d5: *123 | uint64
	e1: *123.456 | float32
	e2: *123.456 | float64
}
