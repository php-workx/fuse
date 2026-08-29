#if canImport(Testing)
import Foundation
import Testing
@testable import FuseNotifierProtocol

@Test func protocolRejectsInvalidUTF8() {
    do {
        try JSONLineCodec.decodeCommand(Data([0x7B, 0xFF, 0x7D]))
        Issue.record("Expected invalid UTF-8 rejection")
    } catch let error as ProtocolError {
        #expect(error == .invalidUTF8)
    } catch {
        Issue.record("Unexpected error: \(error)")
    }
}
#endif
