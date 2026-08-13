import Foundation

final class ConfigStore {
    private let defaults: UserDefaults

    init(defaults: UserDefaults = .standard) {
        self.defaults = defaults
    }

    var configText: String {
        get { defaults.string(forKey: AppConstants.configKey) ?? "" }
        set { defaults.set(newValue, forKey: AppConstants.configKey) }
    }

    func looksLikeConfig(_ text: String) -> Bool {
        let t = text.trimmingCharacters(in: .whitespacesAndNewlines)
        return t.contains("[Interface]") && t.contains("[Peer]") && t.contains("PrivateKey")
    }
}
