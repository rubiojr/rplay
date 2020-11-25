.PHONY: all clean test restic

all: rplay 

rplay: clean
	go build

release:
	./script/build

clean:
	rm -f rplay

test: rplay
	./script/test
