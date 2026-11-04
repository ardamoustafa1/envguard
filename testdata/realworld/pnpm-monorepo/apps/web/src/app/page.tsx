import { Button } from "@acme/ui";
import { listUsers } from "../lib/api";

export default async function Home() {
  const users = await listUsers();

  return (
    <main>
      <h1>Users ({users.length})</h1>
      <ul>
        {users.map((u) => (
          <li key={u.id}>{u.email}</li>
        ))}
      </ul>
      <Button>Invite someone</Button>
    </main>
  );
}
