.PHONY: build clean

build:
	mkdir -p bin
	go build -o bin/vhosts-go
	cp bin/vhosts-go /usr/local/bin/vhosts-go

clean:
	rm -rf bin/
