export default {
  async email(message, env, ctx) {
    const raw = await new Response(message.raw).arrayBuffer();
    const resp = await fetch(env.LOCAL_SERVER_URL + "/api/raw-email", {
      method: "POST",
      headers: {
        "Content-Type": "message/rfc822",
        "X-Webhook-Secret": env.WEBHOOK_SECRET,
      },
      body: raw,
    });
    if (!resp.ok) {
      console.error("Forward failed:", resp.status, await resp.text());
    }
  },
};
