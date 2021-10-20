package bcode

var (
	// ErrAddonExist application is exist
	ErrAddonExist = NewBcode(400, 40002, "addon name already exists")

	ErrAddonRenderFail = NewBcode(500, 40010, "addon render fail")

	ErrAddonApplyFail = NewBcode(500, 40011, "fail to apply addon application")
)
