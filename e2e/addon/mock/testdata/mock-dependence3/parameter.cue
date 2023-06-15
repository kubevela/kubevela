parameter: {
	//+usage=Custom parameter description
	myparam: *"myns" | string
	//+usage=Deploy to specified clusters. Leave empty to deploy to all clusters.
	clusters?: [...string]
}
