import UIKit
import UniformTypeIdentifiers

// Inline share UI for Openmind: receives a URL, text, or image from the share
// sheet, POSTs it to the instance API with the Bearer token, and dismisses.
// Credentials come from the App Group UserDefaults suite, mirrored there by
// the main app's settings screen (lib/settings.ts via ExtensionStorage).
//
// A plain UIViewController is used instead of SLComposeServiceViewController —
// the compose base class eagerly builds its own UI underneath custom views and
// wastes the extension's ~120 MB memory budget.
class ShareViewController: UIViewController {

  private let appGroup = "group.fun.gilla.openmind"

  // Warm palette from packages/ui design tokens.
  private let paper = UIColor(red: 0.957, green: 0.941, blue: 0.902, alpha: 1) // #F4F0E6
  private let ink = UIColor(red: 0.110, green: 0.102, blue: 0.086, alpha: 1) // #1C1A16
  private let inkMuted = UIColor(red: 0.341, green: 0.325, blue: 0.290, alpha: 1) // #57534A
  private let green = UIColor(red: 0.180, green: 0.490, blue: 0.357, alpha: 1) // #2E7D5B
  private let terracotta = UIColor(red: 0.761, green: 0.290, blue: 0.180, alpha: 1) // #C24A2E

  private let card = UIView()
  private let titleLabel = UILabel()
  private let detailLabel = UILabel()
  private let spinner = UIActivityIndicatorView(style: .medium)

  override func viewDidLoad() {
    super.viewDidLoad()
    view.backgroundColor = UIColor.black.withAlphaComponent(0.25)
    buildCard()
    setState(title: "Saving to Openmind…", detail: nil, busy: true)
  }

  override func viewDidAppear(_ animated: Bool) {
    super.viewDidAppear(animated)
    loadSharedContent { [weak self] payload in
      DispatchQueue.main.async { self?.save(payload) }
    }
  }

  // MARK: - Content loading

  private enum SharedPayload {
    case url(URL)
    case text(String)
    case asset(Data, filename: String, mimeType: String)
  }

  /// One office-document format the API accepts. The server sniffs content and
  /// is the authority; these identifiers only decide what the share sheet
  /// offers to hand us.
  private struct DocumentType {
    let type: UTType
    let mimeType: String
    let fallbackExtension: String
  }

  /// Document formats the enrichment pipeline converts. Built with compactMap
  /// so an identifier the OS does not know simply drops out rather than
  /// collapsing to a catch-all that would match every shared file.
  private static let documentTypes: [DocumentType] = {
    let candidates: [(UTType?, String, String)] = [
      (UTType("org.openxmlformats.wordprocessingml.document"),
       "application/vnd.openxmlformats-officedocument.wordprocessingml.document", "docx"),
      (UTType("org.oasis-open.opendocument.text"),
       "application/vnd.oasis.opendocument.text", "odt"),
      (UTType.rtf, "application/rtf", "rtf"),
      (UTType.epub, "application/epub+zip", "epub"),
    ]
    return candidates.compactMap { type, mime, ext in
      guard let type else { return nil }
      return DocumentType(type: type, mimeType: mime, fallbackExtension: ext)
    }
  }()

  /// The document format this provider can supply, if any.
  private func documentType(for provider: NSItemProvider) -> DocumentType? {
    Self.documentTypes.first { provider.hasItemConformingToTypeIdentifier($0.type.identifier) }
  }

  private enum PendingRecord {
    case asset(Data, name: String, mimeType: String)
    case url(String)
    case note(String)
  }

  /// Walks every attachment on every input item (shares can carry mixed
  /// content) and hands back the first image, URL, or plain-text payload found.
  /// Images win over URL/text so Photos/Files shares upload as assets.
  private func loadSharedContent(completion: @escaping (SharedPayload?) -> Void) {
    let providers = (extensionContext?.inputItems as? [NSExtensionItem])?
      .compactMap(\.attachments).flatMap { $0 } ?? []

    guard !providers.isEmpty else {
      completion(nil)
      return
    }

    var foundAsset: SharedPayload?
    var foundURL: URL?
    var foundText: String?
    let group = DispatchGroup()

    for provider in providers {
      if provider.hasItemConformingToTypeIdentifier(UTType.image.identifier) {
        group.enter()
        provider.loadItem(forTypeIdentifier: UTType.image.identifier, options: nil) { [weak self] item, _ in
          defer { group.leave() }
          guard foundAsset == nil, let self else { return }
          if let payload = self.imagePayload(from: item) {
            foundAsset = payload
          }
        }
      } else if let docType = documentType(for: provider) {
        // Checked before the URL and text branches: RTF conforms to public.text,
        // so a plain-text match would otherwise win and save it as a note.
        group.enter()
        provider.loadItem(forTypeIdentifier: docType.type.identifier, options: nil) { [weak self] item, _ in
          defer { group.leave() }
          guard foundAsset == nil, let self else { return }
          if let payload = self.documentPayload(from: item, type: docType) {
            foundAsset = payload
          }
        }
      } else if provider.hasItemConformingToTypeIdentifier(UTType.url.identifier) {
        group.enter()
        provider.loadItem(forTypeIdentifier: UTType.url.identifier, options: nil) { item, _ in
          if let url = item as? URL, foundURL == nil { foundURL = url }
          group.leave()
        }
      } else if provider.hasItemConformingToTypeIdentifier(UTType.plainText.identifier) {
        group.enter()
        provider.loadItem(forTypeIdentifier: UTType.plainText.identifier, options: nil) { item, _ in
          if let text = item as? String, foundText == nil { foundText = text }
          group.leave()
        }
      }
    }

    group.notify(queue: .main) {
      if let foundAsset {
        completion(foundAsset)
      } else if let foundURL {
        completion(.url(foundURL))
      } else if let foundText {
        completion(.text(foundText))
      } else {
        completion(nil)
      }
    }
  }

  /// Matches the API's default ASSETS_MAX_BYTES (10 MiB). The server is still
  /// the authority and answers 413; this bound exists because the extension has
  /// only a ~120 MB memory budget and would be killed loading a large file
  /// before any response could arrive.
  private static let maxDocumentBytes = 10 * 1024 * 1024

  /// Read a shared document as raw bytes. Unlike images these are uploaded
  /// verbatim — no transcoding — because the server sniffs the container to
  /// identify it and converts the text in the enrichment pipeline. Re-encoding
  /// here would destroy exactly the structure anydoc reads.
  ///
  /// A file URL's size is checked before it is read: Data(contentsOf:) would
  /// otherwise pull the whole thing into memory, and an oversized EPUB would
  /// crash the extension rather than surface the server's 413.
  private func documentPayload(from item: NSSecureCoding?, type: DocumentType) -> SharedPayload? {
    var data: Data?
    var filename = "document." + type.fallbackExtension

    if let url = item as? URL {
      if let size = fileSize(of: url), size > Self.maxDocumentBytes {
        return nil
      }
      data = try? Data(contentsOf: url, options: .mappedIfSafe)
      if !url.lastPathComponent.isEmpty {
        filename = url.lastPathComponent
      }
    } else if let raw = item as? Data {
      data = raw
    }

    guard let data, !data.isEmpty, data.count <= Self.maxDocumentBytes else { return nil }
    return .asset(data, filename: filename, mimeType: type.mimeType)
  }

  /// Byte size of a file URL, or nil when it cannot be determined (in which
  /// case the post-read count check still applies).
  private func fileSize(of url: URL) -> Int? {
    (try? url.resourceValues(forKeys: [.fileSizeKey]))?.fileSize
  }

  /// Normalise a share-sheet image item into JPEG bytes. Photos often hand us
  /// a file URL (HEIC) or a UIImage; the API allowlist is jpeg/png/gif/webp/avif,
  /// so JPEG is the safe common denominator and keeps the extension memory budget.
  private func imagePayload(from item: NSSecureCoding?) -> SharedPayload? {
    if let url = item as? URL {
      guard let data = try? Data(contentsOf: url) else { return nil }
      if let image = UIImage(data: data), let jpeg = image.jpegData(compressionQuality: 0.92) {
        let name = url.deletingPathExtension().lastPathComponent
        let filename = (name.isEmpty ? "photo" : name) + ".jpg"
        return .asset(jpeg, filename: filename, mimeType: "image/jpeg")
      }
      // Already a supported raster format (e.g. PNG/JPEG) — pass through.
      let ext = url.pathExtension.lowercased()
      let mime: String?
      switch ext {
      case "jpg", "jpeg": mime = "image/jpeg"
      case "png": mime = "image/png"
      case "gif": mime = "image/gif"
      case "webp": mime = "image/webp"
      default: mime = nil
      }
      if let mime {
        return .asset(data, filename: url.lastPathComponent, mimeType: mime)
      }
      return nil
    }
    if let image = item as? UIImage, let jpeg = image.jpegData(compressionQuality: 0.92) {
      return .asset(jpeg, filename: "photo.jpg", mimeType: "image/jpeg")
    }
    if let data = item as? Data, let image = UIImage(data: data),
       let jpeg = image.jpegData(compressionQuality: 0.92) {
      return .asset(jpeg, filename: "photo.jpg", mimeType: "image/jpeg")
    }
    return nil
  }

  // MARK: - Save

  private func save(_ payload: SharedPayload?) {
    let defaults = UserDefaults(suiteName: appGroup)
    guard
      let instanceUrl = defaults?.string(forKey: "instanceUrl"), !instanceUrl.isEmpty,
      let token = defaults?.string(forKey: "token"), !token.isEmpty
    else {
      finish(success: false, message: "Open Openmind and connect to your instance first.")
      return
    }

    guard let payload else {
      finish(success: false, message: "Nothing shareable found.")
      return
    }

    switch payload {
    case .asset(let data, let filename, let mimeType):
      uploadImage(data: data, filename: filename, mimeType: mimeType, instanceUrl: instanceUrl, token: token)
    case .url(let url):
      postItem(body: ["url": url.absoluteString], pending: .url(url.absoluteString), instanceUrl: instanceUrl, token: token)
    case .text(let text):
      let trimmed = text.trimmingCharacters(in: .whitespacesAndNewlines)
      guard !trimmed.isEmpty else {
        finish(success: false, message: "Nothing shareable found.")
        return
      }
      // Shared text that is itself a bare link is treated as a URL save.
      if let asURL = URL(string: trimmed), let scheme = asURL.scheme,
         ["http", "https"].contains(scheme.lowercased()) {
        postItem(body: ["url": trimmed], pending: .url(trimmed), instanceUrl: instanceUrl, token: token)
      } else {
        postItem(body: ["note": trimmed], pending: .note(trimmed), instanceUrl: instanceUrl, token: token)
      }
    }
  }

  private func postItem(body: [String: String], pending: PendingRecord, instanceUrl: String, token: String) {
    guard let endpoint = URL(string: "\(instanceUrl)/api/items"),
          let payload = try? JSONSerialization.data(withJSONObject: body)
    else {
      finish(success: false, message: "Invalid instance URL.")
      return
    }

    var request = URLRequest(url: endpoint)
    request.httpMethod = "POST"
    request.httpBody = payload
    request.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
    request.setValue("application/json", forHTTPHeaderField: "content-type")
    request.timeoutInterval = 15
    send(request, pending: pending)
  }

  private func uploadImage(data: Data, filename: String, mimeType: String, instanceUrl: String, token: String) {
    guard let endpoint = URL(string: "\(instanceUrl)/api/assets") else {
      finish(success: false, message: "Invalid instance URL.")
      return
    }

    let boundary = "Boundary-\(UUID().uuidString)"
    var body = Data()
    let safeName = filename.replacingOccurrences(of: "\"", with: "")
    body.append("--\(boundary)\r\n".data(using: .utf8)!)
    body.append(
      "Content-Disposition: form-data; name=\"file\"; filename=\"\(safeName)\"\r\n"
        .data(using: .utf8)!
    )
    body.append("Content-Type: \(mimeType)\r\n\r\n".data(using: .utf8)!)
    body.append(data)
    body.append("\r\n--\(boundary)--\r\n".data(using: .utf8)!)

    var request = URLRequest(url: endpoint)
    request.httpMethod = "POST"
    request.httpBody = body
    request.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
    request.setValue("multipart/form-data; boundary=\(boundary)", forHTTPHeaderField: "Content-Type")
    // Photos can be large; give the extension a bit more runway than text saves.
    request.timeoutInterval = 45
    send(request, pending: .asset(data, name: filename, mimeType: mimeType))
  }

  private func send(_ request: URLRequest, pending: PendingRecord) {
    let session = URLSession(configuration: .ephemeral)
    session.dataTask(with: request) { [weak self] _, response, error in
      DispatchQueue.main.async {
        if error != nil {
          self?.persistPending(pending)
          self?.finishOffline()
          return
        }
        let status = (response as? HTTPURLResponse)?.statusCode ?? 0
        switch status {
        case 201:
          self?.finish(success: true, message: "Saved")
        case 401:
          self?.finish(success: false, message: "Token rejected — reconnect in the app.")
        case 415:
          self?.finish(success: false, message: "That image format isn't supported.")
        default:
          self?.finish(success: false, message: "Save failed (HTTP \(status)).")
        }
      }
    }.resume()
  }

  private func finish(success: Bool, message: String) {
    setState(
      title: success ? "Saved to Openmind" : "Couldn't save",
      detail: success ? nil : message,
      busy: false
    )
    titleLabel.textColor = success ? green : terracotta

    // Leave the confirmation up briefly, longer on failure so it's readable.
    DispatchQueue.main.asyncAfter(deadline: .now() + (success ? 0.9 : 2.2)) { [weak self] in
      self?.extensionContext?.completeRequest(returningItems: [], completionHandler: nil)
    }
  }

  private func persistPending(_ record: PendingRecord) {
    guard let defaults = UserDefaults(suiteName: appGroup) else { return }
    var manifest = (try? JSONSerialization.jsonObject(
      with: defaults.data(forKey: "pendingShares") ?? Data()
    )) as? [[String: Any]] ?? []
    let createdAt = Date().timeIntervalSince1970 * 1000

    switch record {
    case .asset(let data, let name, let mimeType):
      guard let container = FileManager.default.containerURL(
        forSecurityApplicationGroupIdentifier: appGroup
      ) else { return }
      let dir = container.appendingPathComponent("pending-shares", isDirectory: true)
      try? FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
      let filename = UUID().uuidString + fileExtension(for: mimeType)
      guard (try? data.write(to: dir.appendingPathComponent(filename))) != nil else { return }
      manifest.append([
        "kind": "asset", "filename": filename, "name": name,
        "mimeType": mimeType, "createdAt": createdAt,
      ])
    case .url(let value):
      manifest.append(["kind": "url", "value": value, "createdAt": createdAt])
    case .note(let value):
      manifest.append(["kind": "note", "value": value, "createdAt": createdAt])
    }

    // Cap at 20 pending shares, dropping the oldest (and their files).
    if manifest.count > 20 {
      let overflow = manifest.count - 20
      if let container = FileManager.default.containerURL(
        forSecurityApplicationGroupIdentifier: appGroup
      ) {
        for old in manifest.prefix(overflow)
        where (old["kind"] as? String) == "asset" {
          if let fn = old["filename"] as? String {
            try? FileManager.default.removeItem(
              at: container.appendingPathComponent("pending-shares").appendingPathComponent(fn)
            )
          }
        }
      }
      manifest = Array(manifest.suffix(20))
    }

    if let data = try? JSONSerialization.data(withJSONObject: manifest) {
      defaults.set(data, forKey: "pendingShares")
    }
  }

  private func fileExtension(for mime: String) -> String {
    switch mime {
    case "image/png": return ".png"
    case "image/gif": return ".gif"
    case "image/webp": return ".webp"
    default: return ".jpg"
    }
  }

  private func finishOffline() {
    setState(title: "Saved offline", detail: "Will sync when you reopen Openmind.", busy: false)
    titleLabel.textColor = green
    DispatchQueue.main.asyncAfter(deadline: .now() + 1.6) { [weak self] in
      self?.extensionContext?.completeRequest(returningItems: [], completionHandler: nil)
    }
  }

  // MARK: - UI

  private func setState(title: String, detail: String?, busy: Bool) {
    titleLabel.text = title
    detailLabel.text = detail
    detailLabel.isHidden = detail == nil
    busy ? spinner.startAnimating() : spinner.stopAnimating()
  }

  private func buildCard() {
    card.backgroundColor = paper
    card.layer.cornerRadius = 16
    card.layer.shadowColor = ink.cgColor
    card.layer.shadowOpacity = 0.18
    card.layer.shadowRadius = 24
    card.layer.shadowOffset = CGSize(width: 0, height: 8)
    card.translatesAutoresizingMaskIntoConstraints = false
    view.addSubview(card)

    titleLabel.font = .systemFont(ofSize: 17, weight: .semibold)
    titleLabel.textColor = ink
    titleLabel.textAlignment = .center

    detailLabel.font = .systemFont(ofSize: 13)
    detailLabel.textColor = inkMuted
    detailLabel.textAlignment = .center
    detailLabel.numberOfLines = 0

    spinner.color = inkMuted
    spinner.hidesWhenStopped = true

    let stack = UIStackView(arrangedSubviews: [spinner, titleLabel, detailLabel])
    stack.axis = .vertical
    stack.spacing = 8
    stack.alignment = .center
    stack.translatesAutoresizingMaskIntoConstraints = false
    card.addSubview(stack)

    NSLayoutConstraint.activate([
      card.centerXAnchor.constraint(equalTo: view.centerXAnchor),
      card.centerYAnchor.constraint(equalTo: view.centerYAnchor),
      card.widthAnchor.constraint(lessThanOrEqualTo: view.widthAnchor, constant: -48),
      card.widthAnchor.constraint(greaterThanOrEqualToConstant: 240),
      stack.topAnchor.constraint(equalTo: card.topAnchor, constant: 24),
      stack.bottomAnchor.constraint(equalTo: card.bottomAnchor, constant: -24),
      stack.leadingAnchor.constraint(equalTo: card.leadingAnchor, constant: 24),
      stack.trailingAnchor.constraint(equalTo: card.trailingAnchor, constant: -24),
    ])
  }
}
