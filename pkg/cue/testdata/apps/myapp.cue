deployment: {
	name:  "myapp"
	port:  8080
	image: "nginx:v1"
	env: [{
		name:  "MYDB"
		value: "true"
	}]
}
route: {
	domain: "www.example.com"
}
