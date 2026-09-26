import AVFoundation
import SwiftUI

/// Camera preview that reports every QR code it reads.
struct QRScannerView: UIViewControllerRepresentable {
    let onCode: (String) -> Void

    func makeUIViewController(context: Context) -> ScannerController {
        let vc = ScannerController()
        vc.onCode = onCode
        return vc
    }

    func updateUIViewController(_ vc: ScannerController, context: Context) {
        vc.onCode = onCode
    }

    final class ScannerController: UIViewController, AVCaptureMetadataOutputObjectsDelegate {
        var onCode: ((String) -> Void)?
        private let session = AVCaptureSession()
        private var preview: AVCaptureVideoPreviewLayer?

        override func viewDidLoad() {
            super.viewDidLoad()
            view.backgroundColor = .black
            guard let device = AVCaptureDevice.default(for: .video),
                  let input = try? AVCaptureDeviceInput(device: device),
                  session.canAddInput(input) else { return }
            // Setup codes can be dense; use a high-resolution preset.
            session.sessionPreset = session.canSetSessionPreset(.hd1920x1080) ? .hd1920x1080 : .high
            session.addInput(input)
            let output = AVCaptureMetadataOutput()
            guard session.canAddOutput(output) else { return }
            session.addOutput(output)
            output.setMetadataObjectsDelegate(self, queue: .main)
            output.metadataObjectTypes = [.qr]

            let layer = AVCaptureVideoPreviewLayer(session: session)
            layer.videoGravity = .resizeAspectFill
            view.layer.addSublayer(layer)
            preview = layer

            if device.isFocusModeSupported(.continuousAutoFocus), (try? device.lockForConfiguration()) != nil {
                device.focusMode = .continuousAutoFocus
                device.unlockForConfiguration()
            }
        }

        override func viewDidLayoutSubviews() {
            super.viewDidLayoutSubviews()
            preview?.frame = view.bounds
        }

        override func viewWillAppear(_ animated: Bool) {
            super.viewWillAppear(animated)
            let session = self.session
            DispatchQueue.global(qos: .userInitiated).async { if !session.isRunning { session.startRunning() } }
        }

        override func viewWillDisappear(_ animated: Bool) {
            super.viewWillDisappear(animated)
            let session = self.session
            DispatchQueue.global(qos: .userInitiated).async { if session.isRunning { session.stopRunning() } }
        }

        func metadataOutput(_ output: AVCaptureMetadataOutput, didOutput metadataObjects: [AVMetadataObject], from connection: AVCaptureConnection) {
            for case let code as AVMetadataMachineReadableCodeObject in metadataObjects {
                if let text = code.stringValue { onCode?(text) }
            }
        }
    }
}

/// Scans one or more setup codes and returns the assembled payload.
struct ScanSetupView: View {
    let onScanned: (SetupPayload) -> Void

    @Environment(\.dismiss) private var dismiss
    @State private var assembler = SetupCodeAssembler()
    @State private var message: String?
    @State private var cameraAllowed = AVCaptureDevice.authorizationStatus(for: .video) == .authorized
    @State private var finished = false

    var body: some View {
        ZStack {
            Color.black.ignoresSafeArea()
            if cameraAllowed {
                QRScannerView(onCode: handle).ignoresSafeArea()
                viewfinder
            } else {
                VStack(spacing: 16) {
                    Text("Camera access is needed to scan the setup code.")
                        .foregroundStyle(.white)
                        .multilineTextAlignment(.center)
                    Button("Open Settings") {
                        if let url = URL(string: UIApplication.openSettingsURLString) { UIApplication.shared.open(url) }
                    }
                    .buttonStyle(PrimaryButtonStyle())
                    .frame(maxWidth: 260)
                }
                .padding(32)
            }
        }
        .safeAreaInset(edge: .bottom) { statusPanel }
        .navigationTitle("Scan setup code")
        .navigationBarTitleDisplayMode(.inline)
        .toolbarBackground(.visible, for: .navigationBar)
        .toolbar {
            ToolbarItem(placement: .cancellationAction) { Button("Cancel") { dismiss() } }
        }
        .task {
            if AVCaptureDevice.authorizationStatus(for: .video) == .notDetermined {
                cameraAllowed = await AVCaptureDevice.requestAccess(for: .video)
            }
        }
    }

    private func handle(_ text: String) {
        guard !finished else { return }
        guard let part = SetupPart.parse(text) else {
            message = "That QR code isn't a Tunnelkey setup code."
            return
        }
        guard assembler.add(part) else { return }
        UIImpactFeedbackGenerator(style: .medium).impactOccurred()
        message = nil
        if assembler.isComplete {
            do {
                let payload = try assembler.payload()
                finished = true
                onScanned(payload)
            } catch {
                message = error.localizedDescription
            }
        }
    }

    private var statusPanel: some View {
        VStack(spacing: 12) {
            if assembler.total > 1 {
                HStack(spacing: 8) {
                    ForEach(1...assembler.total, id: \.self) { i in
                        Circle()
                            .fill(assembler.parts[i] != nil ? Palette.brass : Palette.line)
                            .frame(width: 10, height: 10)
                    }
                }
                Text("\(assembler.parts.count) of \(assembler.total) codes scanned — keep scanning the others")
            } else {
                Text("Point the camera at the QR code from your administrator.")
            }
            if let message {
                Text(message).foregroundStyle(Palette.danger).font(.subheadline)
            }
        }
        .multilineTextAlignment(.center)
        .foregroundStyle(Palette.ink)
        .frame(maxWidth: .infinity)
        .padding(24)
        .background(Palette.raised)
        .accessibilityElement(children: .combine)
    }

    private var viewfinder: some View {
        GeometryReader { geo in
            let side = min(geo.size.width, geo.size.height) * 0.72
            let arm = side * 0.14
            Path { p in
                let r = CGRect(x: (geo.size.width - side) / 2, y: (geo.size.height - side) / 2.4, width: side, height: side)
                for (x, y, dx, dy) in [(r.minX, r.minY, 1.0, 1.0), (r.maxX, r.minY, -1.0, 1.0), (r.minX, r.maxY, 1.0, -1.0), (r.maxX, r.maxY, -1.0, -1.0)] {
                    p.move(to: CGPoint(x: x + arm * dx, y: y))
                    p.addLine(to: CGPoint(x: x, y: y))
                    p.addLine(to: CGPoint(x: x, y: y + arm * dy))
                }
            }
            .stroke(Palette.brass, style: StrokeStyle(lineWidth: 5, lineCap: .round, lineJoin: .round))
        }
        .allowsHitTesting(false)
        .accessibilityHidden(true)
    }
}
