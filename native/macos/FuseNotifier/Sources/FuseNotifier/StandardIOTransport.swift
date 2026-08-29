import Foundation
import FuseNotifierCore
import FuseNotifierProtocol

final class StandardIOTransport: HelperTransport, @unchecked Sendable {
    private let input: FileHandle
    private let output: FileHandle
    private let writeQueue = DispatchQueue(label: "com.php-workx.fuse.notifier.output")
    private let readQueue = DispatchQueue(label: "com.php-workx.fuse.notifier.input")

    init(input: FileHandle = .standardInput, output: FileHandle = .standardOutput) {
        self.input = input
        self.output = output
    }

    func start(onCommand: @escaping @Sendable (HelperCommand) -> Void, onEOF: @escaping @Sendable () -> Void) {
        readQueue.async { [self] in
            var line = Data()
            while let byte = readByte() {
                if byte == 0x0A {
                    if line.last == 0x0D {
                        line.removeLast()
                    }
                    do {
                        onCommand(try JSONLineCodec.decodeCommand(line))
                    } catch {
                        onEOF()
                        return
                    }
                    line.removeAll(keepingCapacity: true)
                    continue
                }

                line.append(byte)
                if line.count > JSONLineCodec.maximumLineBytes {
                    onEOF()
                    return
                }
            }
            onEOF()
        }
    }

    func send(_ event: HelperEvent) throws {
        let line = try JSONLineCodec.encode(event)
        try writeQueue.sync {
            try output.write(contentsOf: line)
        }
    }

    private func readByte() -> UInt8? {
        guard let data = try? input.read(upToCount: 1), let byte = data.first else {
            return nil
        }
        return byte
    }
}
