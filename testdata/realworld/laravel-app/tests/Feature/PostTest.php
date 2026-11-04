<?php

namespace Tests\Feature;

use App\Models\Post;
use Illuminate\Foundation\Testing\RefreshDatabase;
use Tests\TestCase;

class PostTest extends TestCase
{
    use RefreshDatabase;

    public function test_it_lists_published_posts(): void
    {
        Post::factory()->count(2)->create();

        $this->getJson('/api/v1/posts')
            ->assertOk()
            ->assertJsonCount(2, 'data');
    }

    public function test_it_runs_in_the_testing_environment(): void
    {
        $this->assertSame('testing', config('app.env'));
    }
}
