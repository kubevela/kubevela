package test

type Enum struct {
	A string  `json:"a" cue:"enum:abc,def,ghi"`
	B int     `json:"b" cue:"enum:1,2,3"`
	C bool    `json:"c" cue:"enum:true,false"`
	D float64 `json:"d" cue:"enum:1.1,2.2,3.3"`
	E string  `json:"e" cue:"enum:abc,def,ghi;default:ghi"`
	F int     `json:"f" cue:"enum:1,2,3;default:2"`
	G bool    `json:"g" cue:"enum:true,false;default:false"`
	H float64 `json:"h" cue:"enum:1.1,2.2,3.3;default:1.1"` // if default value is first enum, '*' will not be added
	I string  `json:"i" cue:"enum:abc"`
}
