package i18n

// Definitions are all the words and phrases for internationalization in cli and docs
var Definitions = map[string]map[Language]string{
	"Description": {
		Zh: "描述",
		En: "Description",
	},
	"Samples": {
		Zh: "示例",
		En: "Samples",
	},
	"Specification": {
		Zh: "参数说明",
		En: "Specification",
	},
	"AlibabaCloud": {
		Zh: "阿里云",
		En: "Alibaba Cloud",
	},
	"Name": {
		Zh: "名称",
		En: "Name",
	},
	"Type": {
		Zh: "类型",
		En: "Type",
	},
	"Required": {
		Zh: "是否必须",
		En: "Required",
	},
	"Default": {
		Zh: "默认值",
		En: "Default",
	},
	"WriteConnectionSecretToRefIntroduction": {
		Zh: "如果设置了 `writeConnectionSecretToRef`，一个 Kubernetes Secret 将会被创建，并且，它的数据里有这些键（key）：",
		En: "If `writeConnectionSecretToRef` is set, a secret will be generated with these keys as below:",
	},
	"Outputs": {
		Zh: "输出",
		En: "Outputs",
	},
	"Properties": {
		Zh: "属性",
		En: "Properties",
	},
}
