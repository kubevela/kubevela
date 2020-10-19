package template

type FakeTemplateManager struct {
	TraitNames map[string]struct{}
}

func NewFakeTemplateManager() *FakeTemplateManager {
	return &FakeTemplateManager{
		TraitNames: make(map[string]struct{}),
	}
}

func (f *FakeTemplateManager) IsTrait(key string) bool {
	_, ok := f.TraitNames[key]
	return ok
}

func (f *FakeTemplateManager) LoadTemplate(key string) string {
	panic("implement me")
}
