import Foundation

public enum LaunchConfiguration: Equatable, Sendable {
    case inheritedPipes
    case socket(path: String, token: String)

    public static func parse(_ arguments: [String]) throws -> LaunchConfiguration {
        guard !arguments.isEmpty else {
            return .inheritedPipes
        }
        guard arguments.count == 4, arguments[0] == "--connect", arguments[2] == "--token" else {
            throw LaunchConfigurationError.invalidArguments
        }

        let path = arguments[1]
        guard !path.isEmpty, path.utf8.count < 104 else {
            throw LaunchConfigurationError.invalidSocketPath
        }

        let token = arguments[3]
        guard token.utf8.count == 64, token.allSatisfy({ $0.isASCII && $0.isHexDigit }) else {
            throw LaunchConfigurationError.invalidToken
        }
        return .socket(path: path, token: token)
    }
}

public enum LaunchConfigurationError: Error, Equatable, Sendable {
    case invalidArguments
    case invalidSocketPath
    case invalidToken
}
