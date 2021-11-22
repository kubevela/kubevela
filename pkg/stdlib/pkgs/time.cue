#DateToTimestamp: {
	#do:       "timestamp"
	#provider: "time"

	date:   string
	layout: *"" | string

	timestamp?: int64
	...
}

#TimestampToDate: {
	#do:       "date"
	#provider: "time"

	timestamp: int64
	layout:    *"" | string
	location:  *"" | string

	date?: string
	...
}
