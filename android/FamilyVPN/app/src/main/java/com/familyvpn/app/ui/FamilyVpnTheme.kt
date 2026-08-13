package com.familyvpn.app.ui

import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Typography
import androidx.compose.material3.darkColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.sp

object VpnColors {
    val Night = Color(0xFF0B0618)
    val Deep = Color(0xFF120A2A)
    val Indigo = Color(0xFF2A1460)
    val Violet = Color(0xFF7B5CFF)
    val Electric = Color(0xFF5B8CFF)
    val Magenta = Color(0xFFB56CFF)
    val Mist = Color(0xFFC9B8FF)
    val Soft = Color(0xFFEDE6FF)
    val Muted = Color(0xFF9B8FC4)
    val Danger = Color(0xFFFF6B8A)
    val Glass = Color(0x22FFFFFF)
    val GlassBorder = Color(0x33E0D4FF)
}

private val colors = darkColorScheme(
    primary = VpnColors.Violet,
    onPrimary = Color.White,
    secondary = VpnColors.Electric,
    background = VpnColors.Night,
    surface = VpnColors.Deep,
    onBackground = VpnColors.Soft,
    onSurface = VpnColors.Soft,
    error = VpnColors.Danger,
)

private val typography = Typography(
    displayLarge = TextStyle(
        fontFamily = FontFamily.SansSerif,
        fontWeight = FontWeight.SemiBold,
        fontSize = 36.sp,
        letterSpacing = (-0.5).sp,
    ),
    titleMedium = TextStyle(
        fontFamily = FontFamily.SansSerif,
        fontWeight = FontWeight.Medium,
        fontSize = 16.sp,
        letterSpacing = 0.2.sp,
    ),
    bodyMedium = TextStyle(
        fontFamily = FontFamily.SansSerif,
        fontWeight = FontWeight.Normal,
        fontSize = 14.sp,
        lineHeight = 20.sp,
    ),
    labelLarge = TextStyle(
        fontFamily = FontFamily.SansSerif,
        fontWeight = FontWeight.SemiBold,
        fontSize = 15.sp,
        letterSpacing = 0.4.sp,
    ),
)

@Composable
fun FamilyVpnTheme(content: @Composable () -> Unit) {
    MaterialTheme(
        colorScheme = colors,
        typography = typography,
        content = content,
    )
}
