package model

type MenuHint struct {
	Key         string
	Description string
}

func NewMenuHint(key, description string) MenuHint {
	return MenuHint{
		Key:         key,
		Description: description,
	}
}
