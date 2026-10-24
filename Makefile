MAKES-COMMIT := c96aff8f0083d98866b2e4f1d39d79d62f380ba4
M ?= .cache/makes
$(shell test -d $M || { \
  git clone -q https://github.com/makeplus/makes $M && \
  git -C $M checkout -q $(MAKES-COMMIT); \
})
GO-VERSION := 1.27.1
GLOAT-VERSION := 0.1.80
JAVA-VERSION := 27
YAMLSCRIPT-VERSION := 0.2.32
include $M/init.mk
include $M/go.mk
include $M/gh.mk
include $M/gloat.mk
include $M/babashka.mk
include $M/clojure.mk
include $M/lein.mk
include $M/perl.mk
include $M/yamlscript.mk
include $M/clean.mk
include $M/shellcheck.mk
include $M/shell.mk

VERSION := 0.1.0
ifneq (,$(filter release,$(MAKECMDGOALS)))
ifeq (,$(strip $(v)))
$(error 'make release' requires v=VERSION)
endif
endif

MODULE := github.com/yamlstar/yamlstar-plugin-parser-json-comments
MAKES-CLEAN := target pom.xml .cpcache .cache/generated .cache/src
YAML-PARSER-VERSION := 0.2.6
YAML-PARSER-BASE := https://repo.clojars.org/org/yamlstar/yaml-parser
YAML-PARSER-FILE := yaml-parser-$(YAML-PARSER-VERSION).jar
YAML-PARSER-URL := $(YAML-PARSER-BASE)/$(YAML-PARSER-VERSION)/$(YAML-PARSER-FILE)
YAML_PARSER_DIR ?= $(if $(wildcard ../yaml-reference-parser-clj/src),../yaml-reference-parser-clj)
SOURCE := src/yamlstar_plugin/parser/json_comments.cljc
GENERATED := internal/glojure/pkg/yamlstar_plugin/parser/json_comments
BASE-DIR := .cache/src/yaml_parser
BASE-SOURCES := $(addprefix $(BASE-DIR)/,\
  prelude.clj parser.clj receiver.clj grammar.clj canonical.clj core.clj)
SOURCE-STAMP := $(BASE-DIR)/.prepared
JAR := .cache/yaml-parser-$(YAML-PARSER-VERSION).jar
WORK := $(CURDIR)/.cache/development.work
ifneq (,$(wildcard ../yamlstar-plugin-parser-reference/go.mod))
WORK-ENV := GOWORK=$(WORK)
WORK-DEP := $(WORK)
endif

default:: test

$(SOURCE-STAMP): $(wildcard $(YAML_PARSER_DIR)/src/yaml_parser/*.clj*)
	mkdir -p $(BASE-DIR)
ifdef YAML_PARSER_DIR
	cp $(YAML_PARSER_DIR)/src/yaml_parser/*.clj* $(BASE-DIR)/
else
	curl -fsSL $(YAML-PARSER-URL) -o $(JAR)
	unzip -oq $(JAR) 'yaml_parser/*' -d .cache/src
endif
	for file in $(BASE-DIR)/*.cljc; do cp $$file $${file%c}; done
	touch $@

generate: $(SOURCE-STAMP) $(GLOAT)
	rm -rf .cache/generated
	mkdir -p .cache/generated .cache/src
	cp $(SOURCE) .cache/src/json_comments.clj
	env -u GOROOT $(GLOAT) --force --module $(MODULE) \
	  -o .cache/generated/ $(BASE-SOURCES) .cache/src/json_comments.clj
	mkdir -p $(GENERATED)
	cp .cache/generated/pkg/yamlstar_plugin/parser/json_comments/loader.go $(GENERATED)/

check-generated: $(SOURCE-STAMP) $(GLOAT)
	rm -rf .cache/generated
	mkdir -p .cache/generated .cache/src
	cp $(SOURCE) .cache/src/json_comments.clj
	env -u GOROOT $(GLOAT) --force --module $(MODULE) \
	  -o .cache/generated/ $(BASE-SOURCES) .cache/src/json_comments.clj
	diff -ru $(GENERATED) .cache/generated/pkg/yamlstar_plugin/parser/json_comments

$(WORK):
	@mkdir -p .cache
	@printf 'go 1.24.0\n\nuse (\n  ..\n  ../../yamlstar-plugin-parser-reference\n)\n\nreplace github.com/yamlstar/yamlstar-plugin-parser-reference v0.2.6 => ../../yamlstar-plugin-parser-reference\n' > $@

test: test-clojure test-go test-shell

test-shell: $(SHELLCHECK)
	$(SHELLCHECK) util/release

test-clojure: $(SOURCE-STAMP) $(BB)
	$(BB) -cp src:test:$(if $(YAML_PARSER_DIR),$(YAML_PARSER_DIR)/src,.cache/src) \
	  -m yamlstar-plugin.parser.json-comments-test

test-go: $(GO) $(PERL) $(WORK-DEP)
	$(WORK-ENV) $(GO) test ./...

test-race: $(GO) $(PERL) $(WORK-DEP)
	$(WORK-ENV) $(GO) test -race ./...

test-jvm: $(CLOJURE)
	CLJ_CONFIG=$(LOCAL-HOME)/.clojure $(CLOJURE) \
	  -M$(if $(YAML_PARSER_DIR),:dev):test

release-check: test test-jvm test-race check-generated
	grep -Fxq 'const Version = "$(VERSION)"' parser/parser.go
	grep -Fxq '(def version "$(VERSION)")' $(SOURCE)
	grep -Fq '(defproject org.yamlstar/yamlstar-plugin-parser-json-comments "$(VERSION)"' project.clj
	GOWORK=off $(GO) test ./...

release: $(GH) $(LEIN) $(YS) $(PERL)
	RELEASE_VERSION=$(VERSION) \
	RELEASE_OLD_VERSION=$(o) \
	RELEASE_ALLOW_BRANCH=$(a) \
	RELEASE_MAKE='$(MAKE)' \
	RELEASE_REPO=yamlstar/yamlstar-plugin-parser-json-comments \
	RELEASE_DRY_RUN=$(d) \
	GH=$(GH) \
	LEIN=$(LEIN) \
	YS=$(YS) \
	PERL=$(PERL) \
	util/release $(v)
