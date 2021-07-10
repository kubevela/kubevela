package context

type Context interface {
	Clone() Context
	GetComponent(name string,label map[string]string) (Component,error)
	PatchComponent(name string, label map[string]string, patchContent string)error
	GetVar(name string,scope string)(Var,error)
	SetVar(name string,scope string,v Var) error
	Step()(string,int)
	Commit()error
}
