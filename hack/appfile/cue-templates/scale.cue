#scale: {
	replica: *1 | int
	auto: {
		range: string
		cpu:   int
		qps:   int
	}
}

parameter: #scale
