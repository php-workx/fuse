import Foundation

public enum ProtocolError: Error, Equatable, Sendable {
    case lineTooLong
    case invalidUTF8
    case unsupportedVersion(Int)
    case unsupportedCommand(String)
    case invalidRequestID
    case missingField(String)
    case fieldTooLong(String)
    case unsafeField(String)
    case malformedLine
    case encodingTooLong
}

public struct Notice: Equatable, Sendable {
    public let id: String
    public let title: String
    public let subtitle: String?
    public let body: String
    public let sound: Bool

    public init(id: String, title: String, subtitle: String?, body: String, sound: Bool) {
        self.id = id
        self.title = title
        self.subtitle = subtitle
        self.body = body
        self.sound = sound
    }
}

public enum HelperCommand: Equatable, Sendable {
    case hello(requestID: UInt64, clientVersion: String)
    case post(requestID: UInt64, notice: Notice)
    case remove(requestID: UInt64, id: String)
    case reconcile(requestID: UInt64, currentIDs: [String])
    case focus(requestID: UInt64)
    case removeAll(requestID: UInt64)
    case shutdown(requestID: UInt64)

    public var requestID: UInt64 {
        switch self {
        case let .hello(requestID, _), let .post(requestID, _), let .remove(requestID, _), let .reconcile(requestID, _), let .focus(requestID), let .removeAll(requestID), let .shutdown(requestID): requestID
        }
    }

    public var operation: String {
        switch self {
        case .hello: "hello"
        case .post: "post"
        case .remove: "remove"
        case .reconcile: "reconcile"
        case .focus: "focus"
        case .removeAll: "remove_all"
        case .shutdown: "shutdown"
        }
    }
}

public enum HelperEvent: Codable, Equatable, Sendable {
    case ready(requestID: UInt64, helperVersion: String, protocolMin: Int, protocolMax: Int, authorization: String, alertSetting: String, soundSetting: String)
    case result(requestID: UInt64, operation: String, id: String, status: String)
    case posted(requestID: UInt64, id: String)
    case activated(id: String)
    case reconciled(requestID: UInt64, deliveredCurrentIDs: [String])
    case focusResult(requestID: UInt64, mode: String, detail: String)
    case settingsChanged(authorization: String, alertSetting: String, soundSetting: String)
    case error(requestID: UInt64, operation: String, id: String, code: String, message: String)
    case shutdownComplete

    private enum CodingKeys: String, CodingKey { case version, type, requestID = "request_id", helperVersion = "helper_version", protocolMin = "protocol_min", protocolMax = "protocol_max", authorization, alertSetting = "alert_setting", soundSetting = "sound_setting", operation, id, status, deliveredCurrentIDs = "delivered_current_ids", mode, detail, code, message }

    public func encode(to encoder: Encoder) throws {
        var c = encoder.container(keyedBy: CodingKeys.self)
        try c.encode(1, forKey: .version)
        switch self {
        case let .ready(id, version, protocolMin, protocolMax, authorization, alert, sound):
            try c.encode("ready", forKey: .type); try c.encode(id, forKey: .requestID); try c.encode(version, forKey: .helperVersion); try c.encode(protocolMin, forKey: .protocolMin); try c.encode(protocolMax, forKey: .protocolMax); try c.encode(authorization, forKey: .authorization); try c.encode(alert, forKey: .alertSetting); try c.encode(sound, forKey: .soundSetting)
        case let .result(id, operation, noticeID, status):
            try c.encode("result", forKey: .type); try c.encode(id, forKey: .requestID); try c.encode(operation, forKey: .operation); try c.encode(noticeID, forKey: .id); try c.encode(status, forKey: .status)
        case let .posted(id, noticeID):
            try c.encode("posted", forKey: .type); try c.encode(id, forKey: .requestID); try c.encode(noticeID, forKey: .id)
        case let .activated(id): try c.encode("activated", forKey: .type); try c.encode(id, forKey: .id)
        case let .reconciled(id, ids): try c.encode("reconciled", forKey: .type); try c.encode(id, forKey: .requestID); try c.encode(ids, forKey: .deliveredCurrentIDs)
        case let .focusResult(id, mode, detail): try c.encode("focus_result", forKey: .type); try c.encode(id, forKey: .requestID); try c.encode(mode, forKey: .mode); try c.encode(detail, forKey: .detail)
        case let .settingsChanged(authorization, alert, sound): try c.encode("settings_changed", forKey: .type); try c.encode(authorization, forKey: .authorization); try c.encode(alert, forKey: .alertSetting); try c.encode(sound, forKey: .soundSetting)
        case let .error(id, operation, noticeID, code, message): try c.encode("error", forKey: .type); try c.encode(id, forKey: .requestID); try c.encode(operation, forKey: .operation); try c.encode(noticeID, forKey: .id); try c.encode(code, forKey: .code); try c.encode(message, forKey: .message)
        case .shutdownComplete: try c.encode("shutdown_complete", forKey: .type)
        }
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        switch try c.decode(String.self, forKey: .type) {
        case "posted": self = .posted(requestID: try c.decode(UInt64.self, forKey: .requestID), id: try c.decode(String.self, forKey: .id))
        case "activated": self = .activated(id: try c.decode(String.self, forKey: .id))
        case "shutdown_complete": self = .shutdownComplete
        default: throw ProtocolError.malformedLine
        }
    }
}

struct WireCommand: Decodable {
    let version: Int; let type: String; let requestID: UInt64; let clientVersion: String?; let id: String?; let title: String?; let subtitle: String?; let body: String?; let sound: Bool?; let currentIDs: [String]?
    enum CodingKeys: String, CodingKey { case version, type, requestID = "request_id", clientVersion = "client_version", id, title, subtitle, body, sound, currentIDs = "current_ids" }
}
