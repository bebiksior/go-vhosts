.PHONY: build clean

build:
	mkdir -p bin
	go build -o bin/vhosts-go

clean:
	rm -rf bin/
