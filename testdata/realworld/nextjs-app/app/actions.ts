"use server";

import { revalidatePath } from "next/cache";
import { z } from "zod";

import { auth } from "@/lib/auth";
import { db } from "@/lib/db";

const createProjectSchema = z.object({
  name: z.string().trim().min(1).max(80),
});

export async function createProject(formData: FormData) {
  const session = await auth();
  if (!session?.user?.id) throw new Error("Unauthorized");

  const { name } = createProjectSchema.parse({ name: formData.get("name") });
  await db.project.create({ data: { name, ownerId: session.user.id } });
  revalidatePath("/dashboard");
}
