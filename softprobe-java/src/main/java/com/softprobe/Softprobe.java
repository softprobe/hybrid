package com.softprobe;

/**
 * Ergonomic SDK facade for the Softprobe control runtime (see
 * {@code docs/design.md} §3.2). Mirrors the TypeScript and Python counterparts.
 */
public final class Softprobe {
  private static final String DEFAULT_BASE_URL = "https://runtime.softprobe.dev";

  private final Client client;

  public Softprobe() {
    this(resolveBaseUrl(null));
  }

  private static String resolveBaseUrl(String explicit) {
    if (explicit != null && !explicit.isEmpty()) return explicit;
    String env = System.getenv("SOFTPROBE_RUNTIME_URL");
    if (env != null && !env.isEmpty()) return env;
    return DEFAULT_BASE_URL;
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
