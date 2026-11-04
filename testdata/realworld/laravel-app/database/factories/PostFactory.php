<?php

namespace Database\Factories;

use Illuminate\Database\Eloquent\Factories\Factory;

class PostFactory extends Factory
{
    public function definition(): array
    {
        return [
            'title' => fake()->sentence(),
            'body' => fake()->paragraphs(3, true),
            'published_at' => fake()->dateTimeBetween('-1 year'),
        ];
    }
}
