<?php

namespace App\Http\Controllers;

use App\Models\Post;
use Illuminate\Http\JsonResponse;
use Illuminate\Http\Request;

class PostController extends Controller
{
    public function index(): JsonResponse
    {
        $posts = Post::query()
            ->whereNotNull('published_at')
            ->latest('published_at')
            ->paginate(config('app.per_page', 15));

        return response()->json($posts);
    }

    public function show(Post $post): JsonResponse
    {
        abort_unless($post->published_at, 404);

        return response()->json($post);
    }

    public function store(Request $request): JsonResponse
    {
        $data = $request->validate([
            'title' => ['required', 'string', 'max:160'],
            'body' => ['required', 'string'],
        ]);

        $post = $request->user()->posts()->create($data);

        return response()->json($post, 201);
    }
}
