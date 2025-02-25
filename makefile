MAKEFLAGS := --silent --always-make
MAKE_CONC := $(MAKE) -j 128
VERB := $(if $(filter true,$(verb)),-v,)
CLEAR := $(if $(filter false,$(clear)),,$(if $(filter 0,$(MAKELEVEL)),-c,))
GO_SRC := .
GO_FLAGS := -mod=mod
GO_RUN_ARGS := $(GO_FLAGS) $(GO_SRC) $(run)
GO_TEST_FAIL := $(if $(filter false,$(fail)),,-failfast)
GO_TEST_SHORT := $(if $(filter true,$(short)),-short,)
GO_TEST_ARGS := $(GO_SRC) -count=1 $(VERB) $(GO_TEST_FAIL) $(GO_TEST_SHORT) -run="$(run)"
TMP_DIR := .tmp

# Optional dev dependency on Unix: https://github.com/mitranim/gow.
GOW := gow $(CLEAR) $(VERB)

# Optional dev dependency on Windows: https://github.com/mattgreen/watchexec.
WATCH := watchexec $(CLEAR) -d=0 -r -n

# TODO: if appropriate executable does not exist, print install instructions.
ifeq ($(OS),Windows_NT)
	GO_WATCH := $(WATCH) -w=$(GO_SRC) -- go
else
	GO_WATCH := $(GOW) -w=$(GO_SRC)
endif

ifeq ($(OS),Windows_NT)
	RM_DIR = if exist "$(1)" rmdir /s /q "$(1)"
else
	RM_DIR = rm -rf "$(1)"
endif

ifeq ($(verb),true)
	OK = echo [$@] ok
endif

test.w:
	$(GO_WATCH) test $(GO_TEST_ARGS)

test:
	go test $(GO_TEST_ARGS)
	$(OK)

# Used for local testing.
run.w:
	$(GO_WATCH) run $(GO_RUN_ARGS) $(VERB)

# Used for local testing.
run:
	go run $(GO_RUN_ARGS) $(VERB)

lint.w:
	$(GO_WATCH) -- $(MAKE) lint

lint:
	golangci-lint run
	$(OK)

vet.w:
	$(GO_WATCH) vet $(GO_SRC)

vet:
	go vet $(GO_SRC)
	$(OK)

clean:
	$(call RM_DIR,$(TMP_DIR))

install:
	go install $(GO_FLAGS) $(GO_SRC)
