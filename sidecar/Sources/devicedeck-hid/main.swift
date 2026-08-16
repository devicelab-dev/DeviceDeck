import Foundation
import HIDProtocol
import SimCore

// devicedeck-hid — DeviceDeck's input sidecar.
//
// Usage: devicedeck-hid <udid|booted>
//
// Reads binary frames from stdin (format documented on HIDProtocol.Frame)
// and injects them into the simulator through SimulatorKit's host-side HID
// pipeline. Writes "ready" to stdout once injection is possible; all
// diagnostics go to stderr. Exits 0 on stdin EOF (parent closed the pipe).

guard CommandLine.arguments.count == 2 else {
    fatalStartup("usage: devicedeck-hid <udid|booted>")
}
let udid = CommandLine.arguments[1]

let kit = SimKit.load()
guard kit.hasDigitizerPath else {
    // Without the digitizer recipe there is no working tap on Xcode 26 —
    // the mouse-event fallback drops taps or misreads them as Home.
    fatalStartup("digitizer symbols missing — this Xcode is unsupported")
}
guard let client = HIDClient(udid: udid, kit: kit) else {
    fatalStartup("could not attach to simulator \(udid)")
}
let injector = Injector(kit: kit, client: client)

log("attached udid=\(udid) twoFinger=\(kit.mouseTwoFinger != nil) " +
    "hidArb=\(kit.hidArbitrary != nil) legacyBtn=\(kit.legacyButton != nil) " +
    "keyboard=\(kit.keyboard != nil)")
FileHandle.standardOutput.write(Data("ready\n".utf8))

/// Blocking exact-length read; nil on EOF or short read.
func readExact(_ length: Int) -> Data? {
    let data = FileHandle.standardInput.readData(ofLength: length)
    return data.count == length ? data : nil
}

// Frames execute serially on a dedicated queue: gesture recipes sleep
// between steps, and interleaving two injections would corrupt the HID
// stream's touch state.
DispatchQueue.global(qos: .userInteractive).async {
    while true {
        guard let header = readExact(1) else { exit(0) }
        guard let length = Frame.payloadLength(forType: header[0]) else {
            // Unknown type: the stream is misaligned and every subsequent
            // byte would be garbage. Die loudly; the server restarts us.
            log("unknown frame type 0x\(String(header[0], radix: 16)) — exiting")
            exit(2)
        }
        guard let payload = readExact(length) else { exit(0) }
        guard let frame = Frame.parse(type: header[0], payload: payload) else {
            log("dropped malformed frame type=0x\(String(header[0], radix: 16))")
            continue
        }
        injector.handle(frame)
    }
}

RunLoop.main.run()
