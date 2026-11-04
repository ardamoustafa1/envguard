export default function Home() {
  return <p>{process.env.NEXT_PUBLIC_REAL_SITE_NAME} {process.env.REAL_SERVER_SECRET}</p>;
}
