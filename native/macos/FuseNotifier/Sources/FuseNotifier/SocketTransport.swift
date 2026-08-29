import Darwin
import Foundation
import FuseNotifierProtocol

final class SocketTransport: HelperTransport, @unchecked Sendable {
    private let transport: StandardIOTransport

    private init(handle: FileHandle) {
        transport = StandardIOTransport(input: handle, output: handle)
    }

    static func connect(path: String, token: String) throws -> SocketTransport {
        let descriptor = socket(AF_UNIX, SOCK_STREAM, 0)
        guard descriptor >= 0 else {
            throw SocketTransportError.connectionFailed
        }

        var address = sockaddr_un()
        address.sun_family = sa_family_t(AF_UNIX)
        let pathBytes = Array(path.utf8)
        let didCopyPath = withUnsafeMutableBytes(of: &address.sun_path) { destination in
            guard pathBytes.count < destination.count else { return false }
            destination.initializeMemory(as: UInt8.self, repeating: 0)
            pathBytes.withUnsafeBytes { source in
                destination.baseAddress?.copyMemory(from: source.baseAddress!, byteCount: source.count)
            }
            return true
        }
        guard didCopyPath else {
            close(descriptor)
            throw SocketTransportError.connectionFailed
        }

        let connected = withUnsafePointer(to: &address) { pointer in
            pointer.withMemoryRebound(to: sockaddr.self, capacity: 1) {
                Darwin.connect(descriptor, $0, socklen_t(MemoryLayout<sockaddr_un>.size))
            }
        }
        guard connected == 0 else {
            close(descriptor)
            throw SocketTransportError.connectionFailed
        }

        let handle = FileHandle(fileDescriptor: descriptor, closeOnDealloc: true)
        let handshake = Data("token \(token)\n".utf8)
        handle.write(handshake)
        guard try readLine(from: handle, maximumBytes: 80) == Data("ok".utf8) else {
            throw SocketTransportError.authenticationFailed
        }
        return SocketTransport(handle: handle)
    }

    func start(onCommand: @escaping @Sendable (HelperCommand) -> Void, onEOF: @escaping @Sendable () -> Void) {
        transport.start(onCommand: onCommand, onEOF: onEOF)
    }

    func send(_ event: HelperEvent) {
        transport.send(event)
    }

    private static func readLine(from handle: FileHandle, maximumBytes: Int) throws -> Data? {
        var line = Data()
        while let data = try handle.read(upToCount: 1), let byte = data.first {
            if byte == 0x0A {
                return line
            }
            line.append(byte)
            if line.count > maximumBytes {
                throw SocketTransportError.authenticationFailed
            }
        }
        return nil
    }
}

enum SocketTransportError: Error {
    case connectionFailed
    case authenticationFailed
}
