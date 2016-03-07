install: format
	go install github.com/FlushCapacitor/flush-capacitor

format:
	go fmt ./...

deps.fetch:
	go get -u -d github.com/codegangsta/negroni \
	             github.com/davecheney/gpio \
	             github.com/gorilla/mux \
	             github.com/gorilla/websocket \
	             github.com/unrolled/render \
	             gopkg.in/inconshreveable/log15.v2 \
	             gopkg.in/tomb.v2
