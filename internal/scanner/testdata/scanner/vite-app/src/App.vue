<template>
  <p>Don't read process.env.FAKE_IN_TEMPLATE_HTML here</p>
</template>
<script setup lang="ts">
const api = import.meta.env.VITE_REAL_API_URL;
const secret = import.meta.env.REAL_NOT_EXPOSED;
const mode = import.meta.env.MODE;
</script>
