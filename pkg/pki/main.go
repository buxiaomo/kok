package pki

type pki struct {
	path string
}

func New(path string) pki {
	return pki{
		path,
	}
}
