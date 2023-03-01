package test

Enum: {
	a: "abc" | "def" | "ghi"
	b: 1 | 2 | 3
	c: true | false
	d: 1.1 | 2.2 | 3.3
	e: "abc" | "def" | *"ghi"
	f: 1 | *2 | 3
	g: true | *false
	// if default value is first enum, '*' will not be added
	h: 1.1 | 2.2 | 3.3
	i: "abc"
}
