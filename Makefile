.PHONY: help lint test test-unix test-schema test-config test-keychain \
        test-apply test-show test-doctor test-uninstall test-install test-launcher \
        install-dry-run doctor

BEDROCK_CONFIG_PATH ?= $(CURDIR)/bedrock-config.json
export BEDROCK_CONFIG_PATH

help:
	@echo "Juggernaut development targets:"
	@echo ""
	@echo "  make lint             Run shellcheck on all shell scripts"
	@echo "  make test             Run all bash test suites"
	@echo "  make test-unix        Alias for make test"
	@echo "  make test-schema      Run lib/schema.sh tests only"
	@echo "  make test-config      Run lib/config_manager.sh tests only"
	@echo "  make test-keychain    Run lib/keychain.sh tests only"
	@echo "  make test-apply       Run commands/apply.sh tests only"
	@echo "  make test-show        Run commands/show.sh tests only"
	@echo "  make test-doctor      Run commands/doctor.sh tests only"
	@echo "  make test-uninstall   Run commands/uninstall.sh tests only"
	@echo "  make test-install     Run install.sh tests only"
	@echo "  make test-launcher    Run launcher tests only"
	@echo "  make install-dry-run  Preview install.sh changes without writing files"
	@echo "  make doctor           Run juggernaut doctor on the current installation"

lint:
	shellcheck juggernaut install.sh commands/*.sh \
		lib/keychain.sh lib/schema.sh lib/config_manager.sh \
		lib/profile_paths.sh lib/doctor.sh \
		tests/v2/test_*.sh

test: test-schema test-config test-keychain test-apply test-show \
      test-doctor test-uninstall test-install test-launcher

test-unix: test

test-schema:
	bash ./tests/v2/test_schema.sh

test-config:
	bash ./tests/v2/test_config_manager.sh

test-keychain:
	bash ./tests/v2/test_keychain.sh

test-apply:
	bash ./tests/v2/test_apply.sh

test-show:
	bash ./tests/v2/test_show.sh

test-doctor:
	bash ./tests/v2/test_doctor.sh

test-uninstall:
	bash ./tests/v2/test_uninstall.sh

test-install:
	bash ./tests/v2/test_install.sh

test-launcher:
	bash ./tests/v2/test_launcher.sh

install-dry-run:
	bash ./install.sh --dry-run

doctor:
	bash ./juggernaut doctor
