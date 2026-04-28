function cacheControlFor(key) {
  if (key === "latest.json") {
    return "public, max-age=60";
  }
  if (key === "install.sh" || key === "install.ps1") {
    return "public, max-age=300";
  }
  return "public, max-age=3600";
}

export async function onRequestGet({ request, env }) {
  const url = new URL(request.url);
  const routes = {
    "/i/install.sh": {
      key: "install.sh",
      type: "text/x-shellscript; charset=utf-8",
    },
    "/i/install.ps1": {
      key: "install.ps1",
      type: "text/plain; charset=utf-8",
    },
    "/i/latest.json": {
      key: "latest.json",
      type: "application/json; charset=utf-8",
    },
  };

  let item = routes[url.pathname];

  if (
    !item &&
    url.pathname.startsWith("/i/releases/") &&
    url.pathname.endsWith("/manifest.json")
  ) {
    item = {
      key: url.pathname.slice("/i/".length),
      type: "application/json; charset=utf-8",
    };
  }

  if (!item) {
    return new Response("Not found\n", { status: 404 });
  }

  const body = await env.KNOT_RELEASES.get(item.key);
  if (body === null) {
    return new Response("Not found\n", { status: 404 });
  }

  return new Response(body, {
    headers: {
      "content-type": item.type,
      "cache-control": cacheControlFor(item.key),
    },
  });
}
