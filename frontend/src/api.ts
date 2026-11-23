import type { Job, JobSort, JobUpdate, Scan } from "./types";

const JOB_FIELDS = `
  id companyName jobTitle postingId link postedDate retrievedDate
  status lastUpdated notes contactName contactEmail contactPhone salaryRange location
`;

async function gql<T>(query: string, variables: Record<string, unknown>): Promise<T> {
  const res = await fetch("/graphql", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ query, variables }),
  });
  if (!res.ok) {
    throw new Error(`Request failed: ${res.status} ${res.statusText}`);
  }
  const body = (await res.json()) as { data?: T; errors?: { message: string }[] };
  if (body.errors?.length) {
    throw new Error(body.errors.map((e) => e.message).join("; "));
  }
  return body.data as T;
}

export interface JobPage {
  items: Job[];
  totalCount: number;
}

export async function listJobs(sort: JobSort, limit: number, offset: number): Promise<JobPage> {
  const data = await gql<{ jobs: JobPage }>(
    `query($sort: JobSort, $limit: Int, $offset: Int) {
      jobs(sort: $sort, limit: $limit, offset: $offset) { totalCount items { ${JOB_FIELDS} } }
    }`,
    { sort, limit, offset },
  );
  return data.jobs;
}

export async function updateJob(id: string, input: JobUpdate): Promise<Job> {
  const data = await gql<{ updateJob: Job }>(
    `mutation($id: ID!, $input: UpdateJobInput!) {
      updateJob(id: $id, input: $input) { ${JOB_FIELDS} }
    }`,
    { id, input },
  );
  return data.updateJob;
}

const SCAN_FIELDS = `
  id startedAt finishedAt running
  plugins { name status startedAt finishedAt jobsFound jobsAdded error }
`;

/** The scan in progress, or null. */
export async function currentScan(): Promise<Scan | null> {
  const data = await gql<{ currentScan: Scan | null }>(`{ currentScan { ${SCAN_FIELDS} } }`, {});
  return data.currentScan;
}

/** Starts a scan (or returns the one already running). */
export async function startScan(): Promise<Scan> {
  const data = await gql<{ startScan: Scan }>(`mutation { startScan { ${SCAN_FIELDS} } }`, {});
  return data.startScan;
}

/**
 * Follows a scan over the bff's Server-Sent Events endpoint. onUpdate gets the
 * full scan state on every change; the returned promise resolves when the scan
 * finishes and rejects if the connection fails. Abort via signal to stop early.
 */
export async function watchScan(id: string, onUpdate: (scan: Scan) => void, signal: AbortSignal): Promise<void> {
  const res = await fetch("/graphql/stream", {
    method: "POST",
    headers: { "Content-Type": "application/json", Accept: "text/event-stream" },
    body: JSON.stringify({
      query: `subscription($id: ID!) { scanUpdated(id: $id) { ${SCAN_FIELDS} } }`,
      variables: { id },
    }),
    signal,
  });
  if (!res.ok || !res.body) {
    throw new Error(`Scan progress stream failed: ${res.status} ${await res.text()}`);
  }

  const reader = res.body.pipeThrough(new TextDecoderStream()).getReader();
  let buffer = "";
  for (;;) {
    const { value, done } = await reader.read();
    if (done) return;
    buffer += value;

    // SSE messages are separated by a blank line.
    let sep: number;
    while ((sep = buffer.indexOf("\n\n")) >= 0) {
      const message = buffer.slice(0, sep);
      buffer = buffer.slice(sep + 2);

      let event = "message";
      let data = "";
      for (const line of message.split("\n")) {
        if (line.startsWith("event:")) event = line.slice(6).trim();
        else if (line.startsWith("data:")) data += line.slice(5).trim();
      }
      if (event === "complete") return;
      if (event !== "next") continue;

      const body = JSON.parse(data) as { data?: { scanUpdated: Scan }; errors?: { message: string }[] };
      if (body.errors?.length) throw new Error(body.errors.map((e) => e.message).join("; "));
      if (body.data) onUpdate(body.data.scanUpdated);
    }
  }
}
