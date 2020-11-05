.PHONY: all clean test restic

all: rplay 

rplay: clean
	go build

clean:
	rm -f rplay

test: rplay
	./script/test
