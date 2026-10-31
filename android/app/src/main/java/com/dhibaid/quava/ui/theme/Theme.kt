package com.dhibaid.quava.ui.theme

import android.os.Build
import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.dynamicDarkColorScheme
import androidx.compose.material3.dynamicLightColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext

// Every colour role is set on purpose. Material's defaults for the roles left
// unset are purple-tinted, which would leak into cards, dialogs and the drawer.
private val DarkColorScheme = darkColorScheme(
    primary = BananaYellow80,
    onPrimary = BananaInk,
    primaryContainer = Color(0xFF4A3F10),
    onPrimaryContainer = Color(0xFFFFE9A0),
    secondary = Color(0xFFD4C5A0),
    onSecondary = Color(0xFF362F1A),
    secondaryContainer = WarmDarkContainerHighest,
    onSecondaryContainer = WarmDarkOnSurface,
    tertiary = Pink80,
    onTertiary = Color(0xFF4A2531),
    tertiaryContainer = Color(0xFF633B48),
    onTertiaryContainer = Color(0xFFFFD8E4),
    background = WarmDarkBackground,
    onBackground = WarmDarkOnSurface,
    surface = WarmDarkBackground,
    onSurface = WarmDarkOnSurface,
    surfaceVariant = WarmDarkContainerHighest,
    onSurfaceVariant = WarmDarkOnSurfaceVariant,
    surfaceTint = BananaYellow80,
    inverseSurface = WarmDarkOnSurface,
    inverseOnSurface = WarmDarkBackground,
    inversePrimary = BananaAccentLight,
    outline = WarmDarkOutline,
    outlineVariant = BorderWhite,
    scrim = Color.Black,
    surfaceBright = WarmDarkContainerHighest,
    surfaceDim = WarmDarkContainerLowest,
    surfaceContainerLowest = WarmDarkContainerLowest,
    surfaceContainerLow = WarmDarkContainerLow,
    surfaceContainer = WarmDarkContainer,
    surfaceContainerHigh = WarmDarkContainerHigh,
    surfaceContainerHighest = WarmDarkContainerHighest,
)

private val LightColorScheme = lightColorScheme(
    primary = BananaYellow40,
    onPrimary = BananaInk,
    primaryContainer = Color(0xFFFFE9A0),
    onPrimaryContainer = Color(0xFF3B2F00),
    secondary = Color(0xFF6B5E3A),
    onSecondary = Color.White,
    secondaryContainer = WarmLightContainerHighest,
    onSecondaryContainer = WarmLightOnSurface,
    tertiary = Pink40,
    onTertiary = Color.White,
    tertiaryContainer = Color(0xFFFFD8E4),
    onTertiaryContainer = Color(0xFF31101D),
    background = WarmLightBackground,
    onBackground = WarmLightOnSurface,
    surface = WarmLightBackground,
    onSurface = WarmLightOnSurface,
    surfaceVariant = WarmLightContainerHighest,
    onSurfaceVariant = WarmLightOnSurfaceVariant,
    surfaceTint = BananaAccentLight,
    inverseSurface = Color(0xFF32302A),
    inverseOnSurface = Color(0xFFF5F0E6),
    inversePrimary = BananaYellow80,
    outline = WarmLightOutline,
    outlineVariant = BorderBlack,
    scrim = Color.Black,
    surfaceBright = WarmLightBackground,
    surfaceDim = WarmLightContainerHigh,
    surfaceContainerLowest = WarmLightContainerLowest,
    surfaceContainerLow = WarmLightContainerLow,
    surfaceContainer = WarmLightContainer,
    surfaceContainerHigh = WarmLightContainerHigh,
    surfaceContainerHighest = WarmLightContainerHighest,
)

/**
 * The yellow as a text or icon colour. The bright fill yellow has too little
 * contrast on light surfaces, so light mode uses a deeper amber.
 */
@Composable
fun accentColor(): Color =
    if (isSystemInDarkTheme()) BananaYellow80 else BananaAccentLight

@Composable
fun QuavaTheme(
    darkTheme: Boolean = isSystemInDarkTheme(),
    // Dynamic color is available on Android 12+, keep false for asserting our theme.
    dynamicColor: Boolean = false,
    content: @Composable () -> Unit
) {
    val colorScheme = when {
        dynamicColor && Build.VERSION.SDK_INT >= Build.VERSION_CODES.S -> {
            val context = LocalContext.current
            if (darkTheme) dynamicDarkColorScheme(context) else dynamicLightColorScheme(context)
        }

        darkTheme -> DarkColorScheme
        else -> LightColorScheme
    }

    MaterialTheme(
        colorScheme = colorScheme,
        typography = Typography,
        content = content
    )
}
