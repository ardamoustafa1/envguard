import { redirect } from "next/navigation";

import { createProject } from "@/app/actions";
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";

export default async function DashboardPage() {
  const session = await auth();
  if (!session?.user?.id) redirect("/login");

  const projects = await db.project.findMany({
    where: { ownerId: session.user.id },
    orderBy: { createdAt: "desc" },
  });

  return (
    <main>
      <h1>Projects</h1>
      <ul>
        {projects.map((p) => (
          <li key={p.id}>{p.name}</li>
        ))}
      </ul>
      <form action={createProject}>
        <input name="name" required />
        <button type="submit">Create</button>
      </form>
    </main>
  );
}
