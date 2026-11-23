import type { Job, JobSort, JobUpdate } from "./types";

const JOB_FIELDS = `
  id companyName jobTitle postingId link postedDate retrievedDate
  status lastUpdated notes contactName contactEmail contactPhone
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
