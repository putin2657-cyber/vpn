import SwiftUI
import UIKit
import UniformTypeIdentifiers

struct HomeView: View {
    @EnvironmentObject private var tunnel: TunnelController
    @State private var showImporter = false

    var body: some View {
        ZStack {
            AuroraBackground(active: tunnel.state == .connected || tunnel.state == .connecting)
                .ignoresSafeArea()

            ScrollView {
                VStack(spacing: 22) {
                    Text("FAMILY VPN")
                        .font(.system(size: 13, weight: .bold, design: .rounded))
                        .tracking(4)
                        .foregroundStyle(Color(red: 0.79, green: 0.72, blue: 1.0))

                    ConnectionOrb(state: tunnel.state) {
                        Task { await tunnel.toggle() }
                    }
                    .padding(.top, 8)

                    Text(title)
                        .font(.system(size: 34, weight: .semibold))
                        .foregroundStyle(.white)
                        .animation(.easeInOut(duration: 0.25), value: title)

                    Text(subtitle)
                        .font(.system(size: 15))
                        .foregroundStyle(Color(red: 0.61, green: 0.56, blue: 0.77))
                        .multilineTextAlignment(.center)
                        .padding(.horizontal, 8)

                    if let message = tunnel.message, !message.isEmpty {
                        Text(message)
                            .font(.system(size: 13))
                            .foregroundStyle(Color(red: 1.0, green: 0.42, blue: 0.54))
                            .multilineTextAlignment(.center)
                    }

                    glassPanel {
                        Text("Конфиг AmneziaWG")
                            .font(.system(size: 15, weight: .medium))
                            .foregroundStyle(.white)

                        TextEditor(text: $tunnel.draft)
                            .scrollContentBackground(.hidden)
                            .frame(minHeight: 140)
                            .padding(10)
                            .background(Color.black.opacity(0.22))
                            .clipShape(RoundedRectangle(cornerRadius: 16, style: .continuous))
                            .overlay(
                                RoundedRectangle(cornerRadius: 16, style: .continuous)
                                    .stroke(Color.white.opacity(0.18), lineWidth: 1)
                            )
                            .foregroundStyle(.white)

                        HStack(spacing: 10) {
                            ghostButton("Файл") { showImporter = true }
                            ghostButton("Сохранить") { tunnel.saveDraft() }
                        }
                    }

                    HStack(spacing: 10) {
                        ghostButton("Telegram") {
                            UIApplication.shared.open(AppConstants.supportTg)
                        }
                        ghostButton("Кабинет") {
                            UIApplication.shared.open(AppConstants.cabinetURL)
                        }
                    }
                }
                .padding(.horizontal, 24)
                .padding(.vertical, 28)
            }
        }
        .fileImporter(
            isPresented: $showImporter,
            allowedContentTypes: [.plainText, .data, .item],
            allowsMultipleSelection: false
        ) { result in
            guard let url = try? result.get().first else { return }
            let access = url.startAccessingSecurityScopedResource()
            defer { if access { url.stopAccessingSecurityScopedResource() } }
            if let text = try? String(contentsOf: url, encoding: .utf8) {
                tunnel.importText(text)
            }
        }
    }

    private var title: String {
        switch tunnel.state {
        case .connected: return "Защищено"
        case .connecting: return "Соединение…"
        case .error: return "Ошибка"
        case .disconnected: return "Отключено"
        }
    }

    private var subtitle: String {
        switch tunnel.state {
        case .connected: return "трафик идёт через ваш семейный VPS"
        case .connecting: return "поднимаем AmneziaWG туннель"
        case .error: return "проверьте конфиг и попробуйте снова"
        case .disconnected:
            return tunnel.hasConfig
                ? "нажмите на кнопку, чтобы подключиться"
                : "импортируйте .conf, чтобы включить защиту"
        }
    }

    @ViewBuilder
    private func glassPanel<Content: View>(@ViewBuilder content: () -> Content) -> some View {
        VStack(alignment: .leading, spacing: 12) {
            content()
        }
        .padding(16)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(Color.white.opacity(0.08))
        .clipShape(RoundedRectangle(cornerRadius: 22, style: .continuous))
        .overlay(
            RoundedRectangle(cornerRadius: 22, style: .continuous)
                .stroke(Color.white.opacity(0.18), lineWidth: 1)
        )
    }

    private func ghostButton(_ title: String, action: @escaping () -> Void) -> some View {
        Button(action: action) {
            Text(title)
                .font(.system(size: 14, weight: .semibold))
                .foregroundStyle(.white)
                .frame(maxWidth: .infinity)
                .frame(height: 48)
                .background(
                    LinearGradient(
                        colors: [
                            Color(red: 0.48, green: 0.36, blue: 1.0).opacity(0.28),
                            Color(red: 0.36, green: 0.55, blue: 1.0).opacity(0.20),
                        ],
                        startPoint: .leading,
                        endPoint: .trailing
                    )
                )
                .clipShape(RoundedRectangle(cornerRadius: 16, style: .continuous))
                .overlay(
                    RoundedRectangle(cornerRadius: 16, style: .continuous)
                        .stroke(Color.white.opacity(0.18), lineWidth: 1)
                )
        }
        .buttonStyle(.plain)
    }
}

private struct ConnectionOrb: View {
    let state: VpnUiState
    let action: () -> Void

    @State private var pulse = false

    var body: some View {
        Button(action: action) {
            ZStack {
                pulseRing
                coreButton
                Image(systemName: "power")
                    .font(.system(size: 44, weight: .semibold))
                    .foregroundStyle(accent)
            }
            .frame(width: 220, height: 220)
        }
        .buttonStyle(.plain)
        .onAppear { pulse = isActive }
        .onChange(of: state) { _ in pulse = isActive }
    }

    private var isActive: Bool {
        state == .connected || state == .connecting
    }

    private var pulseRing: some View {
        Circle()
            .stroke(accent.opacity(pulse ? 0.0 : 0.55), lineWidth: 2)
            .frame(width: pulse ? 210 : 170, height: pulse ? 210 : 170)
            .animation(
                isActive ? .easeOut(duration: 1.6).repeatForever(autoreverses: false) : .default,
                value: pulse
            )
    }

    private var coreButton: some View {
        Circle()
            .fill(
                LinearGradient(
                    colors: [
                        Color(red: 0.16, green: 0.10, blue: 0.42),
                        Color(red: 0.08, green: 0.06, blue: 0.18),
                    ],
                    startPoint: .topLeading,
                    endPoint: .bottomTrailing
                )
            )
            .frame(width: 148, height: 148)
            .overlay(coreBorder)
            .shadow(color: accent.opacity(0.45), radius: 24)
    }

    private var coreBorder: some View {
        Circle().stroke(
            LinearGradient(
                colors: [accent, Color(red: 0.71, green: 0.42, blue: 1.0)],
                startPoint: .topLeading,
                endPoint: .bottomTrailing
            ),
            lineWidth: 1.5
        )
    }

    private var accent: Color {
        switch state {
        case .connected: return Color(red: 0.36, green: 0.55, blue: 1.0)
        case .connecting: return Color(red: 0.71, green: 0.42, blue: 1.0)
        case .error: return Color(red: 1.0, green: 0.42, blue: 0.54)
        case .disconnected: return Color(red: 0.48, green: 0.36, blue: 1.0)
        }
    }
}

private struct AuroraBackground: View {
    let active: Bool

    var body: some View {
        TimelineView(.animation(minimumInterval: 1 / 30)) { timeline in
            let t = timeline.date.timeIntervalSinceReferenceDate
            Canvas { context, size in
                context.fill(Path(CGRect(origin: .zero, size: size)), with: .color(Color(red: 0.04, green: 0.02, blue: 0.09)))

                let boost: CGFloat = active ? 1.0 : 0.72
                drawBlob(
                    context: &context,
                    center: CGPoint(
                        x: size.width * (0.25 + 0.12 * cos(t / 5)),
                        y: size.height * (0.18 + 0.08 * sin(t / 5))
                    ),
                    color: Color(red: 0.48, green: 0.36, blue: 1.0).opacity(0.45 * boost),
                    radius: size.width * 0.72
                )
                drawBlob(
                    context: &context,
                    center: CGPoint(
                        x: size.width * (0.78 + 0.1 * sin(t / 6)),
                        y: size.height * (0.28 + 0.1 * cos(t / 7))
                    ),
                    color: Color(red: 0.36, green: 0.55, blue: 1.0).opacity(0.35 * boost),
                    radius: size.width * 0.65
                )
                drawBlob(
                    context: &context,
                    center: CGPoint(
                        x: size.width * (0.5 + 0.08 * cos(t / 4.5)),
                        y: size.height * (0.72 + 0.06 * sin(t / 5.5))
                    ),
                    color: Color(red: 0.71, green: 0.42, blue: 1.0).opacity(0.3 * boost),
                    radius: size.width * 0.7
                )
            }
        }
    }

    private func drawBlob(
        context: inout GraphicsContext,
        center: CGPoint,
        color: Color,
        radius: CGFloat
    ) {
        context.fill(
            Path(ellipseIn: CGRect(
                x: center.x - radius,
                y: center.y - radius,
                width: radius * 2,
                height: radius * 2
            )),
            with: .radialGradient(
                Gradient(colors: [color, .clear]),
                center: center,
                startRadius: 0,
                endRadius: radius
            )
        )
    }
}
