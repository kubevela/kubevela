package template

// FakeTemplateManager defines a fake for test
type FakeTemplateManager struct {
	*manager
}

// NewFakeTemplateManager create a fake manager for test
func NewFakeTemplateManager() *FakeTemplateManager {
	return &FakeTemplateManager{
		manager: newManager(),
	}
}
