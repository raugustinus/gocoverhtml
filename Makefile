.PHONY: build install clean

build:
	go build -o gocoverhtml .

install: build
	cp gocoverhtml ~/go/bin/

clean:
	rm -f gocoverhtml
