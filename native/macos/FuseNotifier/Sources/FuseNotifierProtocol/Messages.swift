import Foundation

public enum ProtocolError: Error, Equatable, Sendable {
    case lineTooLong
    case unsupportedVersion(Int)
    case unsupportedCommand(String)
    case invalidRequestID
    case missingField(String)
    case fieldTooLong(String)
    case malformedLine
    case encodingTooLong
}

public enum HelperCommand: Equatable, Sendable {
    case hello(requestID: UInt64, clientVersion: String)
    case post(requestID: UInt64, id: String)

    public var requestID: UInt64 {
        switch self {
        case let .hello(requestID, _), let .post(requestID, _):
            return requestID
        }
    }

    var type: String {
        switch self {
        case .hello:
            return "hello"
        case .post:
            return "post"
        }
    }
}

public enum HelperEvent: Codable, Equatable, Sendable {
    case ready(requestID: UInt64, helperVersion: String)
    case result(requestID: UInt64, operation: String, id: String, status: String)
    case activated(id: String)
    case error(requestID: UInt64, operation: String, id: String, code: String)
    case shutdownComplete

    private enum CodingKeys: String, CodingKey {
        case version
        case type
        case requestID = "request_id"
        case helperVersion = "helper_version"
        case operation
        case id
        case status
        case code
    }

    public func encode(to encoder: Encoder) throws {
        var container = encoder.container(keyedBy: CodingKeys.self)
        try container.encode(1, forKey: .version)

        switch self {
        case let .ready(requestID, helperVersion):
            try container.encode("ready", forKey: .type)
            try container.encode(requestID, forKey: .requestID)
            try container.encode(helperVersion, forKey: .helperVersion)
        case let .result(requestID, operation, id, status):
            try container.encode("result", forKey: .type)
            try container.encode(requestID, forKey: .requestID)
            try container.encode(operation, forKey: .operation)
            try container.encode(id, forKey: .id)
            try container.encode(status, forKey: .status)
        case let .activated(id):
            try container.encode("activated", forKey: .type)
            try container.encode(id, forKey: .id)
        case let .error(requestID, operation, id, code):
            try container.encode("error", forKey: .type)
            try container.encode(requestID, forKey: .requestID)
            try container.encode(operation, forKey: .operation)
            try container.encode(id, forKey: .id)
            try container.encode(code, forKey: .code)
        case .shutdownComplete:
            try container.encode("shutdown_complete", forKey: .type)
        }
    }

    public init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        let type = try container.decode(String.self, forKey: .type)

        switch type {
        case "ready":
            self = .ready(
                requestID: try container.decode(UInt64.self, forKey: .requestID),
                helperVersion: try container.decode(String.self, forKey: .helperVersion)
            )
        case "result":
            self = .result(
                requestID: try container.decode(UInt64.self, forKey: .requestID),
                operation: try container.decode(String.self, forKey: .operation),
                id: try container.decode(String.self, forKey: .id),
                status: try container.decode(String.self, forKey: .status)
            )
        case "activated":
            self = .activated(id: try container.decode(String.self, forKey: .id))
        case "error":
            self = .error(
                requestID: try container.decode(UInt64.self, forKey: .requestID),
                operation: try container.decode(String.self, forKey: .operation),
                id: try container.decode(String.self, forKey: .id),
                code: try container.decode(String.self, forKey: .code)
            )
        case "shutdown_complete":
            self = .shutdownComplete
        default:
            throw ProtocolError.unsupportedCommand(type)
        }
    }
}

struct WireCommand: Decodable {
    let version: Int
    let type: String
    let requestID: UInt64
    let clientVersion: String?
    let id: String?

    enum CodingKeys: String, CodingKey {
        case version
        case type
        case requestID = "request_id"
        case clientVersion = "client_version"
        case id
    }
}
