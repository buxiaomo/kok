package kibana

import (
	"bytes"
	"fmt"
	"net/http"
)

type index struct {
	url string
}

func (app kibana) Index() index {
	return index{
		app.url,
	}
}

func (app index) Create() {
	posturl := fmt.Sprintf("%s/api/index_patterns/index_pattern", app.url)

	r, err := http.NewRequest("POST", posturl, bytes.NewBuffer([]byte(`{
  "override": false,
  "refresh_fields": true,
  "index_pattern": {
     "title": "hello"
  }
}`)))
	if err != nil {
		panic(err)
	}
	client := &http.Client{}
	res, err := client.Do(r)
	if err != nil {
		panic(err)
	}
	defer res.Body.Close()
	var body []byte
	res.Body.Read(body)
	fmt.Println(string(body))
}
