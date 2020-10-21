package template

type FakeTemplateManager struct {
	*manager
}

func NewFakeTemplateManager() *FakeTemplateManager {
	return &FakeTemplateManager{
		manager: newManager(),
	}
}
