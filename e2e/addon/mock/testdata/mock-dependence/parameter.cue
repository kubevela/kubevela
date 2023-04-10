// parameter.cue is used to store addon parameters.
//
// You can use these parameters in template.cue or in resources/ by 'parameter.myparam'
//
// For example, you can use parameters to allow the user to customize
// container images, ports, and etc.
parameter: {
	// +usage=Custom parameter description
	myparam: *"myns" | string
        //+usage=Deploy to specified clusters. Leave empty to deploy to all clusters.
	clusters?: [...string]
}
