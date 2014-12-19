install: format
	go install github.com/tchap/go-pi-indicator

format:
	go fmt ./...

deps.fetch:
	go get -u -d github.com/codegangsta/negroni
	go get -u -d github.com/davecheney/gpio
	go get -u -d github.com/gorilla/websocket
	go get -u -d github.com/unrolled/render
