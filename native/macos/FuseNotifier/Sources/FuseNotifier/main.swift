import AppKit
import Darwin
import FuseNotifierCore

let transport: any HelperTransport
do {
    switch try LaunchConfiguration.parse(Array(CommandLine.arguments.dropFirst())) {
    case .inheritedPipes:
        transport = StandardIOTransport()
    case let .socket(path, token):
        transport = try SocketTransport.connect(path: path, token: token)
    }
} catch {
    FileHandle.standardError.write(Data("FuseNotifier could not establish its requested transport.\n".utf8))
    exit(64)
}

let application = NSApplication.shared
let delegate = AppDelegate(transport: transport)
application.delegate = delegate
application.setActivationPolicy(.accessory)
application.run()
