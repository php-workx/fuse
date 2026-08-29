import Foundation

public struct ProcessInvocation: Equatable, Sendable {
    public let executable: String
    public let arguments: [String]
    public let timeout: TimeInterval

    public init(executable: String, arguments: [String], timeout: TimeInterval) {
        self.executable = executable
        self.arguments = arguments
        self.timeout = timeout
    }
}

public enum ProcessResult: Equatable, Sendable {
    case success(output: String)
    case failure
}

public protocol ProcessRunning: Sendable {
    func run(_ invocation: ProcessInvocation) -> ProcessResult
}

public struct SystemProcessRunner: ProcessRunning {
    public init() {}

    public func run(_ invocation: ProcessInvocation) -> ProcessResult {
        let process = Process()
        let output = Pipe()
        let completed = DispatchSemaphore(value: 0)
        process.terminationHandler = { _ in completed.signal() }
        process.executableURL = URL(fileURLWithPath: invocation.executable)
        process.arguments = invocation.arguments
        process.standardOutput = output
        process.standardError = output

        do {
            try process.run()
        } catch {
            return .failure
        }
        if completed.wait(timeout: .now() + invocation.timeout) == .timedOut {
            process.terminate()
            _ = completed.wait(timeout: .now() + 1)
            return .failure
        }

        guard process.terminationStatus == 0 else { return .failure }
        let data = output.fileHandleForReading.readDataToEndOfFile()
        guard let text = String(data: data, encoding: .utf8) else { return .failure }
        return .success(output: text)
    }
}
