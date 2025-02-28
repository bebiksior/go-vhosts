.PHONY: build clean

build:
	mkdir -p bin
	go build -o bin/go-vhosts cmd/go-vhosts/main.go
	cp bin/go-vhosts /usr/local/bin/go-vhosts

clean:
	rm -rf bin/
