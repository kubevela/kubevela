package stdlib

type discover struct {
	files []file
}

// Pkgs is map[${path}]${package-content}
type Pkgs map[string]string

func (p *discover) packages() Pkgs {
	pkgs := map[string]string{}
	for _, f := range p.files {
		pkgs[f.path] += f.content + "\n"
	}
	return pkgs
}

func (p *discover) addFile(f file) {
	p.files = append(p.files, f)
}

// GetPackages Get Stdlib packages
func GetPackages() Pkgs {
	d := &discover{}
	d.addFile(opFile)
	return d.packages()
}
