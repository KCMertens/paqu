
include ../Makefile.cfg

../../bin/pqbugtest: *.go
	CGO_LDFLAGS="-Wl,-rpath=$(LIBRARY_PATH)" go build -o $@ $(TAGS) $(OPTS) .
