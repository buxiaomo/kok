package kibana

type kibana struct {
	url string
}

func New(url string) kibana {
	return kibana{
		url,
	}
}
