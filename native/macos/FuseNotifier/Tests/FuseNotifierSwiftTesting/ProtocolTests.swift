#if canImport(Testing)
import Foundation
import Testing
@testable import FuseNotifierProtocol

@Test func protocolRejectsInvalidUTF8() {
    do {
        _ = try JSONLineCodec.decodeCommand(Data([0x7B, 0xFF, 0x7D]))
        Issue.record("Expected invalid UTF-8 rejection")
    } catch let error as ProtocolError {
        #expect(error == .invalidUTF8)
    } catch {
        Issue.record("Unexpected error: \(error)")
    }
}

@Test func helperEventsRoundTrip() throws {
    let events: [HelperEvent] = [
        .ready(requestID: 1, helperVersion: "test", protocolMin: 1, protocolMax: 1, pid: 123, authorization: "authorized", alertSetting: "enabled", soundSetting: "enabled"),
        .result(requestID: 2, operation: "post", id: "id", status: "accepted"),
        .posted(requestID: 2, id: "id"),
        .activated(id: "id"),
        .reconciled(requestID: 3, deliveredCurrentIDs: ["id"]),
        .focusResult(requestID: 4, mode: "exact", detail: "focused"),
        .settingsChanged(authorization: "denied", alertSetting: "disabled", soundSetting: "disabled"),
        .error(requestID: 5, operation: "post", id: "id", code: "failed", message: ""),
        .shutdownComplete,
    ]
    for event in events {
        #expect(try JSONDecoder().decode(HelperEvent.self, from: JSONEncoder().encode(event)) == event)
    }
}
@Test func helperEventsRequireProtocolVersion() {
    for frame in [#"{"type":"activated","id":"id"}"#, #"{"version":2,"type":"activated","id":"id"}"#] {
        do {
            _ = try JSONDecoder().decode(HelperEvent.self, from: Data(frame.utf8))
            Issue.record("Expected event version rejection")
        } catch {}
    }
}
#endif
