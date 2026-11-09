// Keep transport failures in the same shape as API validation errors.
export async function api(path, body) {
  try {
    const response = await fetch(
      "/api" + path,
      body === undefined
        ? {}
        : {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify(body),
          },
    );
    if (response.status === 401) {
      return {
        error:
          "Authentication required. Open this page with ?token=YOUR_TOKEN.",
      };
    }
    const data = await response.json();
    if (!response.ok)
      return { error: data.error || `Request failed (${response.status}).` };
    return data;
  } catch {
    return {
      error:
        "Cannot reach memdroid. Check that the server is running, then try again.",
    };
  }
}
