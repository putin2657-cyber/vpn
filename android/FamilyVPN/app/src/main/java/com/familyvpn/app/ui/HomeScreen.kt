package com.familyvpn.app.ui

import androidx.compose.animation.AnimatedContent
import androidx.compose.animation.animateColorAsState
import androidx.compose.animation.core.FastOutSlowInEasing
import androidx.compose.animation.core.LinearEasing
import androidx.compose.animation.core.RepeatMode
import androidx.compose.animation.core.animateFloat
import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.animation.core.infiniteRepeatable
import androidx.compose.animation.core.rememberInfiniteTransition
import androidx.compose.animation.core.spring
import androidx.compose.animation.core.tween
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.scaleIn
import androidx.compose.animation.slideInVertically
import androidx.compose.animation.slideOutVertically
import androidx.compose.animation.togetherWith
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.interaction.collectIsPressedAsState
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.OutlinedTextFieldDefaults
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.alpha
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.drawBehind
import androidx.compose.ui.draw.scale
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.familyvpn.app.vpn.VpnUiState
import kotlin.math.cos
import kotlin.math.sin

@Composable
fun HomeScreen(
    vpnState: VpnUiState,
    hasConfig: Boolean,
    draft: String,
    message: String?,
    onDraftChange: (String) -> Unit,
    onSaveConfig: () -> Unit,
    onPickFile: () -> Unit,
    onToggle: () -> Unit,
    onSupport: () -> Unit,
    onCabinet: () -> Unit,
) {
    val connected = vpnState == VpnUiState.Connected
    val connecting = vpnState == VpnUiState.Connecting
    val errored = vpnState == VpnUiState.Error

    val title = when (vpnState) {
        VpnUiState.Connected -> "Защищено"
        VpnUiState.Connecting -> "Соединение…"
        VpnUiState.Error -> "Ошибка"
        VpnUiState.Disconnected -> "Отключено"
    }
    val sub = when {
        !hasConfig -> "импортируйте .conf, чтобы включить защиту"
        connected -> "трафик идёт через ваш семейный VPS"
        connecting -> "поднимаем AmneziaWG туннель"
        errored -> "проверьте конфиг и попробуйте снова"
        else -> "нажмите на кнопку, чтобы подключиться"
    }

    Box(modifier = Modifier.fillMaxSize()) {
        AnimatedAuroraBackground(active = connected || connecting)

        Column(
            modifier = Modifier
                .fillMaxSize()
                .verticalScroll(rememberScrollState())
                .padding(horizontal = 24.dp, vertical = 28.dp),
            horizontalAlignment = Alignment.CenterHorizontally,
        ) {
            Text(
                text = "FAMILY VPN",
                color = VpnColors.Mist,
                fontSize = 13.sp,
                fontWeight = FontWeight.Bold,
                letterSpacing = 4.sp,
            )

            Spacer(Modifier.height(28.dp))

            ConnectionOrb(
                state = vpnState,
                enabled = !connecting && (hasConfig || draft.isNotBlank() || connected),
                onClick = onToggle,
            )

            Spacer(Modifier.height(28.dp))

            AnimatedContent(
                targetState = title,
                transitionSpec = {
                    (fadeIn(tween(280)) + slideInVertically { it / 3 } + scaleIn(initialScale = 0.96f)) togetherWith
                        (fadeOut(tween(180)) + slideOutVertically { -it / 4 })
                },
                label = "statusTitle",
            ) { text ->
                Text(
                    text = text,
                    color = VpnColors.Soft,
                    fontSize = 34.sp,
                    fontWeight = FontWeight.SemiBold,
                    textAlign = TextAlign.Center,
                )
            }

            Spacer(Modifier.height(8.dp))

            AnimatedContent(
                targetState = sub,
                transitionSpec = {
                    fadeIn(tween(320)) togetherWith fadeOut(tween(160))
                },
                label = "statusSub",
            ) { text ->
                Text(
                    text = text,
                    color = VpnColors.Muted,
                    fontSize = 15.sp,
                    textAlign = TextAlign.Center,
                    modifier = Modifier.padding(horizontal = 12.dp),
                )
            }

            if (!message.isNullOrBlank()) {
                Spacer(Modifier.height(12.dp))
                Text(
                    text = message,
                    color = VpnColors.Danger,
                    fontSize = 13.sp,
                    textAlign = TextAlign.Center,
                )
            }

            Spacer(Modifier.height(28.dp))

            GlassPanel {
                Text(
                    text = "Конфиг AmneziaWG",
                    color = VpnColors.Soft,
                    fontWeight = FontWeight.Medium,
                    fontSize = 15.sp,
                )
                Spacer(Modifier.height(12.dp))
                OutlinedTextField(
                    value = draft,
                    onValueChange = onDraftChange,
                    modifier = Modifier
                        .fillMaxWidth()
                        .height(150.dp),
                    placeholder = {
                        Text("Вставьте содержимое .conf", color = VpnColors.Muted.copy(alpha = 0.7f))
                    },
                    colors = OutlinedTextFieldDefaults.colors(
                        focusedTextColor = VpnColors.Soft,
                        unfocusedTextColor = VpnColors.Soft,
                        focusedBorderColor = VpnColors.Violet.copy(alpha = 0.7f),
                        unfocusedBorderColor = VpnColors.GlassBorder,
                        cursorColor = VpnColors.Electric,
                        focusedContainerColor = Color(0x22000000),
                        unfocusedContainerColor = Color(0x18000000),
                    ),
                    shape = RoundedCornerShape(16.dp),
                )
                Spacer(Modifier.height(12.dp))
                Row(
                    horizontalArrangement = Arrangement.spacedBy(10.dp),
                    modifier = Modifier.fillMaxWidth(),
                ) {
                    GhostButton("Файл", Modifier.weight(1f), onPickFile)
                    GhostButton("Сохранить", Modifier.weight(1f), onSaveConfig)
                }
            }

            Spacer(Modifier.height(14.dp))

            Row(
                horizontalArrangement = Arrangement.spacedBy(10.dp),
                modifier = Modifier.fillMaxWidth(),
            ) {
                GhostButton("Telegram", Modifier.weight(1f), onSupport)
                GhostButton("Кабинет", Modifier.weight(1f), onCabinet)
            }

            Spacer(Modifier.height(20.dp))
        }
    }
}

@Composable
private fun AnimatedAuroraBackground(active: Boolean) {
    val infinite = rememberInfiniteTransition(label = "aurora")
    val drift by infinite.animateFloat(
        initialValue = 0f,
        targetValue = 360f,
        animationSpec = infiniteRepeatable(
            animation = tween(18000, easing = LinearEasing),
            repeatMode = RepeatMode.Restart,
        ),
        label = "drift",
    )
    val pulse by infinite.animateFloat(
        initialValue = 0.55f,
        targetValue = 0.95f,
        animationSpec = infiniteRepeatable(
            animation = tween(4200, easing = FastOutSlowInEasing),
            repeatMode = RepeatMode.Reverse,
        ),
        label = "pulse",
    )
    val activeBoost by animateFloatAsState(
        targetValue = if (active) 1f else 0.72f,
        animationSpec = tween(700),
        label = "boost",
    )

    Canvas(modifier = Modifier.fillMaxSize()) {
        drawRect(VpnColors.Night)

        val w = size.width
        val h = size.height
        val rad = Math.toRadians(drift.toDouble())
        val cx1 = w * (0.25f + 0.12f * cos(rad).toFloat())
        val cy1 = h * (0.18f + 0.08f * sin(rad).toFloat())
        val cx2 = w * (0.78f + 0.1f * sin(rad).toFloat())
        val cy2 = h * (0.28f + 0.1f * cos(rad * 0.8).toFloat())
        val cx3 = w * (0.5f + 0.08f * cos(rad * 1.3).toFloat())
        val cy3 = h * (0.72f + 0.06f * sin(rad * 1.1).toFloat())

        drawCircle(
            brush = Brush.radialGradient(
                colors = listOf(
                    VpnColors.Violet.copy(alpha = 0.55f * pulse * activeBoost),
                    Color.Transparent,
                ),
                center = Offset(cx1, cy1),
                radius = w * 0.72f,
            ),
            center = Offset(cx1, cy1),
            radius = w * 0.72f,
        )
        drawCircle(
            brush = Brush.radialGradient(
                colors = listOf(
                    VpnColors.Electric.copy(alpha = 0.42f * activeBoost),
                    Color.Transparent,
                ),
                center = Offset(cx2, cy2),
                radius = w * 0.65f,
            ),
            center = Offset(cx2, cy2),
            radius = w * 0.65f,
        )
        drawCircle(
            brush = Brush.radialGradient(
                colors = listOf(
                    VpnColors.Magenta.copy(alpha = 0.35f * pulse * activeBoost),
                    Color.Transparent,
                ),
                center = Offset(cx3, cy3),
                radius = w * 0.7f,
            ),
            center = Offset(cx3, cy3),
            radius = w * 0.7f,
        )
        drawRect(
            brush = Brush.verticalGradient(
                colors = listOf(Color.Transparent, VpnColors.Night.copy(alpha = 0.55f)),
            ),
        )
    }
}

@Composable
private fun ConnectionOrb(
    state: VpnUiState,
    enabled: Boolean,
    onClick: () -> Unit,
) {
    val connected = state == VpnUiState.Connected
    val connecting = state == VpnUiState.Connecting
    val errored = state == VpnUiState.Error

    val infinite = rememberInfiniteTransition(label = "orb")
    val ring1 by infinite.animateFloat(
        initialValue = 0.9f,
        targetValue = 1.18f,
        animationSpec = infiniteRepeatable(
            animation = tween(1600, easing = FastOutSlowInEasing),
            repeatMode = RepeatMode.Restart,
        ),
        label = "ring1",
    )
    val ringAlpha by infinite.animateFloat(
        initialValue = 0.55f,
        targetValue = 0f,
        animationSpec = infiniteRepeatable(
            animation = tween(1600, easing = LinearEasing),
            repeatMode = RepeatMode.Restart,
        ),
        label = "ringA",
    )
    val spin by infinite.animateFloat(
        initialValue = 0f,
        targetValue = 360f,
        animationSpec = infiniteRepeatable(
            animation = tween(1400, easing = LinearEasing),
            repeatMode = RepeatMode.Restart,
        ),
        label = "spin",
    )

    val accent by animateColorAsState(
        targetValue = when {
            connected -> VpnColors.Electric
            connecting -> VpnColors.Magenta
            errored -> VpnColors.Danger
            else -> VpnColors.Violet
        },
        animationSpec = tween(400),
        label = "accent",
    )

    val interaction = remember { MutableInteractionSource() }
    val pressed by interaction.collectIsPressedAsState()
    val pressScale by animateFloatAsState(
        targetValue = if (pressed) 0.94f else 1f,
        animationSpec = spring(dampingRatio = 0.55f, stiffness = 500f),
        label = "press",
    )
    val idleBreath by infinite.animateFloat(
        initialValue = 1f,
        targetValue = 1.03f,
        animationSpec = infiniteRepeatable(
            animation = tween(2400, easing = FastOutSlowInEasing),
            repeatMode = RepeatMode.Reverse,
        ),
        label = "breath",
    )
    val scale = pressScale * if (connected) idleBreath else 1f

    Box(
        contentAlignment = Alignment.Center,
        modifier = Modifier
            .size(220.dp)
            .scale(scale)
            .alpha(if (enabled) 1f else 0.45f)
            .clickable(
                enabled = enabled,
                interactionSource = interaction,
                indication = null,
                onClick = onClick,
            ),
    ) {
        if (connected || connecting) {
            Canvas(modifier = Modifier.fillMaxSize()) {
                val canvas = this.size
                val r = canvas.minDimension / 2f
                drawCircle(
                    color = accent.copy(alpha = ringAlpha * 0.7f),
                    radius = r * ring1 * 0.78f,
                    style = Stroke(width = 3.dp.toPx()),
                )
                drawCircle(
                    color = accent.copy(alpha = ringAlpha * 0.4f),
                    radius = r * (ring1 * 0.9f + 0.08f) * 0.78f,
                    style = Stroke(width = 2.dp.toPx()),
                )
            }
        }

        if (connecting) {
            Canvas(modifier = Modifier.size(188.dp)) {
                drawArc(
                    brush = Brush.sweepGradient(
                        listOf(Color.Transparent, accent, VpnColors.Electric, Color.Transparent),
                    ),
                    startAngle = spin,
                    sweepAngle = 110f,
                    useCenter = false,
                    style = Stroke(width = 4.dp.toPx(), cap = StrokeCap.Round),
                )
            }
        }

        Box(
            contentAlignment = Alignment.Center,
            modifier = Modifier
                .size(148.dp)
                .drawBehind {
                    val glow = this.size.minDimension * 0.72f
                    drawCircle(
                        brush = Brush.radialGradient(
                            colors = listOf(accent.copy(alpha = 0.45f), Color.Transparent),
                        ),
                        radius = glow,
                    )
                }
                .clip(CircleShape)
                .background(
                    Brush.linearGradient(
                        colors = listOf(
                            Color(0xFF2A1A6B),
                            Color(0xFF1A1040),
                            Color(0xFF14102E),
                        ),
                    ),
                )
                .border(
                    width = 1.5.dp,
                    brush = Brush.linearGradient(
                        listOf(accent.copy(alpha = 0.95f), VpnColors.Magenta.copy(alpha = 0.55f)),
                    ),
                    shape = CircleShape,
                ),
        ) {
            Canvas(modifier = Modifier.size(54.dp)) {
                val stroke = 5.dp.toPx()
                val canvas = this.size
                val cx = canvas.width / 2f
                val cy = canvas.height / 2f
                val r = canvas.minDimension / 2f - stroke
                drawArc(
                    color = accent,
                    startAngle = -210f,
                    sweepAngle = 240f,
                    useCenter = false,
                    topLeft = Offset(cx - r, cy - r + 4.dp.toPx()),
                    size = androidx.compose.ui.geometry.Size(r * 2, r * 2),
                    style = Stroke(width = stroke, cap = StrokeCap.Round),
                )
                drawLine(
                    color = accent,
                    start = Offset(cx, cy - r + 2.dp.toPx()),
                    end = Offset(cx, cy + 4.dp.toPx()),
                    strokeWidth = stroke,
                    cap = StrokeCap.Round,
                )
            }
        }
    }
}

@Composable
private fun GlassPanel(content: @Composable () -> Unit) {
    Column(
        modifier = Modifier
            .fillMaxWidth()
            .clip(RoundedCornerShape(22.dp))
            .background(VpnColors.Glass)
            .border(1.dp, VpnColors.GlassBorder, RoundedCornerShape(22.dp))
            .padding(16.dp),
        content = { content() },
    )
}

@Composable
private fun GhostButton(
    label: String,
    modifier: Modifier = Modifier,
    onClick: () -> Unit,
) {
    val interaction = remember { MutableInteractionSource() }
    val pressed by interaction.collectIsPressedAsState()
    val scale by animateFloatAsState(
        targetValue = if (pressed) 0.97f else 1f,
        animationSpec = spring(stiffness = 600f),
        label = "ghostPress",
    )

    Box(
        contentAlignment = Alignment.Center,
        modifier = modifier
            .height(48.dp)
            .scale(scale)
            .clip(RoundedCornerShape(16.dp))
            .background(
                Brush.horizontalGradient(
                    listOf(
                        VpnColors.Violet.copy(alpha = 0.18f),
                        VpnColors.Electric.copy(alpha = 0.14f),
                    ),
                ),
            )
            .border(1.dp, VpnColors.GlassBorder, RoundedCornerShape(16.dp))
            .clickable(
                interactionSource = interaction,
                indication = null,
                onClick = onClick,
            ),
    ) {
        Text(
            text = label,
            color = VpnColors.Soft,
            fontWeight = FontWeight.SemiBold,
            fontSize = 14.sp,
        )
    }
}
