import Foundation
import FuseNotifierProtocol

func expect(_ condition: @autoclosure () -> Bool, _ message: String) { if !condition() { fatalError(message) } }
func expectError(_ expected: ProtocolError, _ operation: () throws -> Void) {
    do { try operation(); fatalError("Expected \(expected)") }
    catch let actual as ProtocolError { expect(actual == expected, "Expected \(expected), received \(actual)") }
    catch { fatalError("Unexpected error: \(error)") }
}
func decode(_ line: String) throws -> HelperCommand { try JSONLineCodec.decodeCommand(Data(line.utf8)) }

do {
    let post = try decode(#"{"version":1,"type":"post","request_id":2,"id":"approval-1","title":"untrusted","subtitle":"ignored","body":"untrusted","sound":true}"#)
    expect(post == .post(requestID: 2, notice: Notice(id: "approval-1", title: "untrusted", subtitle: "ignored", body: "untrusted", sound: true)), "Post did not decode")
    let remove = try decode(#"{"version":1,"type":"remove","request_id":3,"id":"approval-1"}"#)
    let reconcile = try decode(#"{"version":1,"type":"reconcile","request_id":4,"current_ids":["approval-1"]}"#)
    let focus = try decode(#"{"version":1,"type":"focus","request_id":5}"#)
    let removeAll = try decode(#"{"version":1,"type":"remove_all","request_id":6}"#)
    let shutdown = try decode(#"{"version":1,"type":"shutdown","request_id":7}"#)
    expect(remove == .remove(requestID: 3, id: "approval-1"), "Remove did not decode")
    expect(reconcile == .reconcile(requestID: 4, currentIDs: ["approval-1"]), "Reconcile did not decode")
    expect(focus == .focus(requestID: 5), "Focus did not decode")
    expect(removeAll == .removeAll(requestID: 6), "Remove-all did not decode")
    expect(shutdown == .shutdown(requestID: 7), "Shutdown did not decode")
    expectError(.invalidUTF8) { _ = try JSONLineCodec.decodeCommand(Data([0x7B, 0xFF, 0x7D])) }
    expectError(.lineTooLong) { _ = try JSONLineCodec.decodeCommand(Data(repeating: 0x20, count: JSONLineCodec.maximumLineBytes + 1)) }
    expectError(.unsafeField("title")) { _ = try decode(#"{"version":1,"type":"post","request_id":2,"id":"id","title":"line\nfeed","body":"body","sound":false}"#) }
    expectError(.unsafeField("title")) { _ = try decode(#"{"version":1,"type":"post","request_id":2,"id":"id","title":"\u202E","body":"body","sound":false}"#) }
    let title = String(repeating: "a", count: JSONLineCodec.maximumDisplayScalars + 1)
    let line = try JSONSerialization.data(withJSONObject: ["version": 1, "type": "post", "request_id": 2, "id": "id", "title": title, "body": "body", "sound": false])
    expectError(.fieldTooLong("title")) { _ = try JSONLineCodec.decodeCommand(line) }
    let encoded = try JSONLineCodec.encode(.posted(requestID: 2, id: "approval-1"))
    expect(encoded.last == 0x0A && encoded.count <= JSONLineCodec.maximumLineBytes, "Events must be bounded newline-delimited JSON")
} catch { fatalError("Protocol behavior test failed: \(error)") }
