BUILD_DIR := android/quava/build/Qt_6_10_3_for_Android_arm64_v8a_Debug
QT_ANDROIDDEPLOYQT := /home/dhibaid/hdd/QT/6.10.3/gcc_64/bin/androiddeployqt
JDK := /usr/lib/jvm/java-21-openjdk
PACKAGE := org.qtproject.example.appquava

APK := $(BUILD_DIR)/android-build-appquava/build/outputs/apk/debug/android-build-appquava-debug.apk

.PHONY: build apk install run clean

build:
	cmake --build $(BUILD_DIR) --target all

apk: build
	JAVA_HOME=$(JDK) PATH=$(JDK)/bin:$$PATH $(QT_ANDROIDDEPLOYQT) \
		--input $(BUILD_DIR)/android-appquava-deployment-settings.json \
		--output $(BUILD_DIR)/android-build-appquava \
		--android-platform android-36 \
		--jdk $(JDK) \
		--gradle

install: apk
	@devices=$$(adb devices | awk 'NR > 1 && /[[:space:]]device[[:space:]]*$$/ { sub(/[[:space:]]device[[:space:]]*$$/, ""); print }'); \
	if [ -z "$$devices" ]; then \
		echo "No devices connected."; \
		exit 1; \
	fi; \
	echo "Select device:"; \
	select device in $$devices; do \
		if [ -n "$$device" ]; then \
			echo "Installing on device: $$device"; \
			adb -s "$$device" uninstall "$(PACKAGE)" >/dev/null 2>&1 || true; \
			adb -s "$$device" install -r "$(APK)"; \
			break; \
		else \
			echo "Invalid selection."; \
		fi; \
	done
run: install
	adb shell monkey -p $(PACKAGE) 1

clean:
	rm -rf $(BUILD_DIR)
