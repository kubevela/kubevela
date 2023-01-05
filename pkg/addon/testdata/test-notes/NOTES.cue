info: string

if !context.installer.upgrade {
	info: "first installation!"
}
if context.installer.upgrade {
	info: "upgrade!"
}

notes: "Thank you for your " + """
\(info)
Please refer to URL.
"""
