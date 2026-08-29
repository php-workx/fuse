import Foundation

public enum JSONLineCodec {
    public static let maximumLineBytes = 32 * 1024
    public static let maximumDisplayScalars = 160

    public static func decodeCommand(_ line: Data) throws -> HelperCommand {
        guard line.count <= maximumLineBytes else {
            throw ProtocolError.lineTooLong
        }

        let wire: WireCommand
        do {
            wire = try JSONDecoder().decode(WireCommand.self, from: line)
        } catch {
            throw ProtocolError.malformedLine
        }

        guard wire.version == 1 else {
            throw ProtocolError.unsupportedVersion(wire.version)
        }
        guard wire.requestID > 0 else {
            throw ProtocolError.invalidRequestID
        }

        switch wire.type {
        case "hello":
            guard let clientVersion = wire.clientVersion else {
                throw ProtocolError.missingField("client_version")
            }
            try validateScalarCount(clientVersion, field: "client_version")
            return .hello(requestID: wire.requestID, clientVersion: clientVersion)
        case "post":
            guard let id = wire.id, !id.isEmpty else {
                throw ProtocolError.missingField("id")
            }
            try validateScalarCount(id, field: "id")
            return .post(requestID: wire.requestID, id: id)
        default:
            throw ProtocolError.unsupportedCommand(wire.type)
        }
    }

    public static func encode(_ event: HelperEvent) throws -> Data {
        var encoded = try JSONEncoder().encode(event)
        guard encoded.count < maximumLineBytes else {
            throw ProtocolError.encodingTooLong
        }
        encoded.append(0x0A)
        return encoded
    }

    private static func validateScalarCount(_ value: String, field: String) throws {
        guard value.unicodeScalars.count <= maximumDisplayScalars else {
            throw ProtocolError.fieldTooLong(field)
        }
    }
}
