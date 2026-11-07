plugins {
    alias(libs.plugins.android.application)
    alias(libs.plugins.kotlin.compose)
}

android {
    namespace = "com.dhibaid.quava"
    compileSdk {
        version = release(37)
    }

    defaultConfig {
        applicationId = "com.dhibaid.quava"
        minSdk = 24
        targetSdk = 37
        // Overridable by CI via -PquavaVersionCode/-PquavaVersionName so release
        // builds carry the tag's version instead of this fallback.
        versionCode = (project.findProperty("quavaVersionCode") as String?)?.toInt() ?: 1
        versionName = project.findProperty("quavaVersionName") as String? ?: "1.0"

        testInstrumentationRunner = "androidx.test.runner.AndroidJUnitRunner"
    }

    // Populated from env vars in CI (see .github/workflows/release.yml). Left
    // unset for local builds, where only assembleDebug is normally used.
    signingConfigs {
        create("release") {
            System.getenv("QUAVA_RELEASE_KEYSTORE")?.let { storeFile = file(it) }
            storePassword = System.getenv("QUAVA_RELEASE_KEYSTORE_PASSWORD")
            keyAlias = System.getenv("QUAVA_RELEASE_KEY_ALIAS")
            keyPassword = System.getenv("QUAVA_RELEASE_KEY_PASSWORD")
        }
    }

    buildTypes {
        release {
            optimization {
                enable = false
            }
            signingConfig = signingConfigs.getByName("release")
        }
    }
    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_11
        targetCompatibility = JavaVersion.VERSION_11
    }
    buildFeatures {
        compose = true
    }
}

dependencies {
    implementation("com.upokecenter:cbor:4.5.2")
    implementation("org.bouncycastle:bcprov-jdk18on:1.78.1")
    implementation(platform(libs.androidx.compose.bom))
    implementation(libs.androidx.activity.compose)
    implementation(libs.androidx.compose.material3)
    implementation(libs.androidx.compose.ui)
    implementation(libs.androidx.compose.ui.graphics)
    implementation(libs.androidx.compose.ui.tooling.preview)
    implementation(libs.androidx.core.ktx)
    implementation(libs.androidx.lifecycle.runtime.ktx)
    implementation("androidx.compose.material:material-icons-extended")
    implementation("androidx.media:media:1.8.0")
    implementation("io.coil-kt:coil-compose:2.7.0")
    implementation("com.github.mwiede:jsch:0.2.23")
    testImplementation(libs.junit)
    androidTestImplementation(platform(libs.androidx.compose.bom))
    androidTestImplementation(libs.androidx.compose.ui.test.junit4)
    androidTestImplementation(libs.androidx.espresso.core)
    androidTestImplementation(libs.androidx.junit)
    debugImplementation(libs.androidx.compose.ui.test.manifest)
    debugImplementation(libs.androidx.compose.ui.tooling)
}
