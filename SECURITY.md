# Security Model & Architecture Overview

CUPS webui (`cups-webui`) is designed as an unprivileged, local-only daemon and web management interface for macOS CUPS (`127.0.0.1:6310`). Because it runs locally and interacts with system printing utilities, it implements defense-in-depth security controls to protect the user against malicious websites, cross-origin web threats, and command injection.

---

## Reporting a Vulnerability

Please report suspected security vulnerabilities privately through PaperCut's security contact page: <https://www.papercut.com/contact/security/>. Do not open a public GitHub issue for security problems.

Include the affected version or commit, steps to reproduce, and the impact you observed.

---

## 1. Loopback Isolation & Web Perimeter Defense

* **Loopback Binding by Default:** The HTTP server binds to `127.0.0.1` by default and is intended to be used on the loopback interface only. Binding it to another interface with `-host` is not a supported configuration.
* **DNS Rebinding Protection:** Every incoming HTTP request must supply a valid loopback `Host` header (`127.0.0.1`, `localhost`, or `[::1]`). Requests routed via external hostnames or DNS rebinding domains are terminated with `400 Bad Request`.
* **CSRF & Cross-Site Defense:** 
  * Every API request (reads as well as `POST`, `PUT`, `DELETE`) is checked against browser metadata (`Sec-Fetch-Site`). Requests a browser marks as coming from another site, or from another `localhost` port, are refused with `403 Forbidden`, so other web pages can't make the server run CUPS commands. Clients that don't send the header (curl, scripts) are handled by the token and connection-owner rules.
  * Origin headers must match the exact address the interface is served on (same host and port). Pages on other loopback ports are treated as cross-origin: they receive no CORS access and cannot obtain the session token.
* **CORS Lockdown on Event Streams:** Server-Sent Events (`/api/v1/events`) do not permit wildcard or cross-origin access, preventing malicious third-party websites from eavesdropping on print jobs, document titles, or queue state.

---

## 2. Authentication & Session Management

* **Cryptographic Session Tokens:** A 128-bit session token is generated using `crypto/rand` and stored at `~/.cups-webui/session.json`.
* **Filesystem Permissions:** The user configuration directory is restricted to `0700` and the session credential file is restricted to `0600` (read/write by the user only).
* **Same-User Sign-In:** The session is established automatically only for connections that originate from the same macOS user account that runs `cups-webui`. Connections from other accounts or other hosts must already hold the session token, for both reading and changing data.
* **Constant-Time Verification:** Session tokens are validated using `crypto/subtle.ConstantTimeCompare` to mitigate timing side-channel attacks.
* **Protected Cookies:** Direct browser navigations issue an `HttpOnly`, `SameSite=Strict`, `Path=/` session cookie only when the navigation origin is verified.
* **Header-Gated Token Handshake:** Session bootstrap endpoints require custom client headers (`X-Requested-With: CUPS-WebUI`), which standard browsers prohibit cross-origin scripts from attaching without explicit CORS approval.

---

## 3. Subprocess Execution & Command Sanitization

`cups-webui` acts as a management proxy over CUPS command-line utilities (`lpadmin`, `lpstat`, `lpoptions`, `cancel`, `cupsctl`). All subprocess invocations adhere to strict security boundaries:

* **No Shell Interpolation:** All commands execute via discrete `argv` arrays directly through `os/exec` (never through `/bin/sh` or a shell interpreter).
* **Strict Parameter Whitelisting:**
  * **Job IDs:** Must be a number or `<printer>-<number>` with a valid printer name, to prevent flag injection (e.g. `cancel -a` or `-u root`).
  * **Printer Names:** Enforced to the CUPS naming limits (1–127 characters; no spaces, control characters, `/`, `\`, `?`, quotes or `#`), and may not start with `-`. The optional alphanumeric-only "Safe Queue Naming" in the Add Printer screen is a recommendation, not a server-side limit.
  * **Device URIs:** Whitelisted strictly to authorized network and hardware transport schemes (`ipp`, `ipps`, `http`, `https`, `socket`, `lpd`, `usb`, `smb`, `dnssd`, `mdns`). Arbitrary schemes like `file://` or `exec://` are prohibited; the one exception is the exact null device `file:/dev/null` (also accepted as `file:///dev/null`), used for virtual queues that discard jobs. A URI may carry the `papercut:` prefix that PaperCut print tracking adds to a queue; the address after the prefix must pass the same checks, so the prefix cannot be used to reach a blocked scheme or path. URIs must not start with a dash or contain control characters, percent-encoding must be well formed, and the value is passed to `lpadmin` unchanged as a single argument (never URL-decoded or interpreted by a shell).
  * **PPD Options:** Option keys and values are sanitized to disallow leading dashes and command separators.
* **Execution Timeouts:** All external CLI processes run within a `context.WithTimeout` (maximum 10 seconds) to prevent hung child processes.

---

## 4. PPD Driver & Filter Hardening

* **System Path Whitelisting:** PPD file selection is restricted to recognized system driver locations (`/Library/Printers/`, `/System/Library/Printers/`, the CUPS model directories under `/usr/share`, `/usr/local/share` and `/opt/homebrew/share`, and `/etc/cups/ppd/`). Temporary and home-directory locations are rejected.
* **Directive Inspection:** Uploaded PPD files are inspected to block known dangerous directives, including `*FoomaticRIPCommandLine` (CVE-2024-47176), and to warn about filter paths outside the standard system locations.
