package bcode

var (

	// ErrAddonNotExist addon not exist
	ErrAddonNotExist = NewBcode(400, 40001, "addon not exist")

	// ErrAddonExist application is exist
	ErrAddonExist = NewBcode(400, 40002, "addon name already exists")

	// ErrAddonRenderFail fail to render addon application
	ErrAddonRenderFail = NewBcode(500, 40010, "addon render fail")

	// ErrAddonApplyFail fail to apply application to cluster
	ErrAddonApplyFail = NewBcode(500, 40011, "fail to apply addon application")

	// ErrGetClientFail fail to get k8s client
	ErrGetClientFail = NewBcode(500, 40012, "fail to initialize kubernetes client")

	// ErrGetApplicationFail fail to get addon application
	ErrGetApplicationFail = NewBcode(500, 40013, "fail to get addon application")

	// ErrGetConfigMapAddonFail fail to get addon info in configmap
	ErrGetConfigMapAddonFail = NewBcode(500, 40014, "fail to get addon information in ConfigMap")

	// ErrAddonDisableFail fail to disable addon
	ErrAddonDisableFail = NewBcode(500, 40016, "fail to disable addon")

	// ErrAddonNotEnabled means addon can't be disable because it's not enabled
	ErrAddonNotEnabled = NewBcode(400, 40017, "addon not enabled")
)
