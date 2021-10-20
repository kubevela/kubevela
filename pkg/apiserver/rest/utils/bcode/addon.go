package bcode

var (
	// ErrAddonExist application is exist
	ErrAddonExist = NewBcode(400, 40002, "addon name is exist")
)
