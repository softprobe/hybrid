package com.softprobe;

/**
 * Ergonomic SDK facade for the Softprobe control runtime (see
 * {@code docs/design.md} §3.2). Mirrors the TypeScript and Python counterparts.
 */
public final class Softprobe {
  private static final String DEFAULT_BASE_URL = "http://127.0.0.1:8080";

  private final Client client;

  public Softprobe() {
    this(DEFAULT_BASE_URL);
  }

  public Softprobe(String baseUrl) {
    this(new Client(baseUrl));
  }

  public Softprobe(String baseUrl, Client.Transport transport) {
    this(new Client(baseUrl, transport));
  }

  /**
   * Creates a facade that attaches {@code Authorization: Bearer <apiToken>} on every
   * runtime call. When {@code apiToken} is {@code null} or blank the
   * {@code SOFTPROBE_API_TOKEN} environment variable is used as a fallback.
   */
  public static Softprobe withApiToken(String baseUrl, String apiToken) {
    return new Softprobe(Client.withApiToken(baseUrl, apiToken));
  }

  public Softprobe(String baseUrl, Client.Transport transport, String apiToken) {
    this(new Client(baseUrl, transport, apiToken));
  }

  /** Package-private constructor for tests that want to inject a pre-built Client. */
  Softprobe(Client client) {
    this.client = client;
  }

  /** Creates a new session and returns a {@link SoftprobeSession} bound to it. */
  public SoftprobeSession startSession(String mode) {
    Object sessionId = client.sessions().create(mode).get("sessionId");
    if (!(sessionId instanceof String)) {
      throw new SoftprobeRuntimeException(
          0, "runtime did not return a sessionId in the create-session response");
    }
    return new SoftprobeSession((String) sessionId, client);
  }

  /** Re-binds an existing session by id, e.g. across processes. */
  public SoftprobeSession attach(String sessionId) {
    return new SoftprobeSession(sessionId, client);
  }
}
