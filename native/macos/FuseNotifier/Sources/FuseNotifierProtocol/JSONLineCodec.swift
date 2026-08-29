import Foundation

public enum JSONLineCodec {
    public static let maximumLineBytes = 32 * 1024
    public static let maximumDisplayScalars = 160

    public static func decodeCommand(_ line: Data) throws -> HelperCommand {
        guard line.count <= maximumLineBytes else { throw ProtocolError.lineTooLong }
        guard String(data: line, encoding: .utf8) != nil else { throw ProtocolError.invalidUTF8 }
        let wire: WireCommand
        do { wire = try JSONDecoder().decode(WireCommand.self, from: line) }
        catch { throw ProtocolError.malformedLine }
        guard wire.version == 1 else { throw ProtocolError.unsupportedVersion(wire.version) }
        guard wire.requestID > 0 else { throw ProtocolError.invalidRequestID }

        switch wire.type {
        case "hello":
            let clientVersion = try required(wire.clientVersion, field: "client_version")
            try validate(clientVersion, field: "client_version", display: true)
            return .hello(requestID: wire.requestID, clientVersion: clientVersion)
        case "post":
            let id = try required(wire.id, field: "id")
            let title = try required(wire.title, field: "title")
            let body = try required(wire.body, field: "body")
            try validate(id, field: "id", display: false)
            try validate(title, field: "title", display: true)
            if let subtitle = wire.subtitle { try validate(subtitle, field: "subtitle", display: true) }
            try validate(body, field: "body", display: true)
            return .post(requestID: wire.requestID, notice: Notice(id: id, title: title, subtitle: wire.subtitle, body: body, sound: wire.sound ?? false))
        case "remove":
            let id = try required(wire.id, field: "id")
            try validate(id, field: "id", display: false)
            return .remove(requestID: wire.requestID, id: id)
        case "reconcile":
            let ids = try required(wire.currentIDs, field: "current_ids")
            for id in ids { try validate(id, field: "current_ids", display: false) }
            return .reconcile(requestID: wire.requestID, currentIDs: ids)
        case "focus": return .focus(requestID: wire.requestID)
        case "remove_all": return .removeAll(requestID: wire.requestID)
        case "shutdown": return .shutdown(requestID: wire.requestID)
        default: throw ProtocolError.unsupportedCommand(wire.type)
        }
    }

    public static func encode(_ event: HelperEvent) throws -> Data {
        var encoded = try JSONEncoder().encode(event)
        guard encoded.count < maximumLineBytes else { throw ProtocolError.encodingTooLong }
        encoded.append(0x0A)
        return encoded
    }

    private static func required<T>(_ value: T?, field: String) throws -> T {
        guard let value else { throw ProtocolError.missingField(field) }
        return value
    }

    private static func validate(_ value: String, field: String, display: Bool) throws {
        guard !value.isEmpty else { throw ProtocolError.missingField(field) }
        if display && value.unicodeScalars.count > maximumDisplayScalars { throw ProtocolError.fieldTooLong(field) }
        for scalar in value.unicodeScalars where isUnsafe(scalar) { throw ProtocolError.unsafeField(field) }
    }

    private static func isUnsafe(_ scalar: Unicode.Scalar) -> Bool {
        switch scalar.value {
        case 0 ... 31, 127 ... 159, 0x202A ... 0x202E, 0x2066 ... 0x2069: true
        default: false
        }
    }
}
