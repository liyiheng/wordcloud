extern crate image;
extern crate rand;
extern crate rusttype;

use image::{DynamicImage, Rgba};
use image::GenericImage;
use rand::prelude::*;
use rusttype::{Font, point, Scale};
use std::io;

/// Example
/// ```
/// extern crate image;
/// extern crate rand;
/// extern crate rusttype;
///
/// use rusttype::Font;
/// mod canvas;
///
///
/// fn main() {
///     let font_data = include_bytes!("../fonts/wqy-microhei/WenQuanYiMicroHei.ttf");
///     let font = Font::from_bytes(font_data as &[u8]).expect("Error constructing Font");
///     let mut canvas = canvas::Canvas::new(font, 640, 480);
///     canvas.default_color = (180,50,0);
///     canvas.draw_text("ArchLinux", 80.0);
///     canvas.draw_text("liyiheng", 42.0);
///     canvas.draw_text("Git", 40.0);
///     canvas.draw_text("Rust", 70.0);
///     canvas.draw_text("中文", 50.0);
///     // Save the image to a png file
///     canvas.save("image_example.png").unwrap();
///     println!("Generated: image_example.png");
/// }
/// ```
pub struct Canvas<'a> {
    font: Font<'a>,
    image: DynamicImage,
    pub default_color: (u8, u8, u8),
}

impl<'a> Canvas<'a> {
    pub fn new(f: Font, w: u32, h: u32) -> Canvas {
        Canvas {
            font: f,
            image: DynamicImage::new_rgba8(w, h),
            default_color: (160, 0, 0),
        }
    }

    pub fn draw_text(&mut self, text: &str, size: f32) {
        let c = self.default_color;
        self.draw_text_with_color(text, size, c);
    }

    pub fn save(&self, path: &str) -> io::Result<()> {
        self.image.save(path)
    }

    pub fn draw_text_with_color(&mut self, text: &str, size: f32, color: (u8, u8, u8)) {

        // The font size to use
        let scale = Scale::uniform(size);

        let v_metrics = self.font.v_metrics(scale);

        // layout the glyphs in a line with 20 pixels padding
        let glyphs: Vec<_> = self.font
            .layout(text, scale, point(0.0, 0.0 + v_metrics.ascent))
            .collect();


        // work out the layout size
        let glyphs_height = (v_metrics.ascent - v_metrics.descent).ceil() as u32;
        let glyphs_width = {
            let min_x = glyphs
                .first()
                .map(|g| g.pixel_bounding_box().unwrap().min.x)
                .unwrap();
            let max_x = glyphs
                .last()
                .map(|g| g.pixel_bounding_box().unwrap().max.x)
                .unwrap();
            (max_x - min_x) as u32
        };
        let point = self.find_blank(glyphs_width, glyphs_height, 10);
        if point.0 < 0 || point.1 < 0 {
            println!("no space left for {}", text);
            return;
        }
        let img = &mut self.image;
        // Loop through the glyphs in the text, positing each one on a line
        for glyph in glyphs {
            if let Some(bounding_box) = glyph.pixel_bounding_box() {
                // Draw the glyph into the image per-pixel by using the draw closure
                glyph.draw(|x, y, v| {
                    img.put_pixel(
                        // Offset the position by the glyph bounding box
                        point.0 as u32 + x + bounding_box.min.x as u32,
                        point.1 as u32 + y + bounding_box.min.y as u32,
                        // Turn the coverage into an alpha value
                        Rgba {
                            data: [color.0, color.1, color.2, (v * 255.0) as u8],
                        },
                    )
                });
            }
        }
    }

    fn find_blank_with_skip(&self, w: u32, h: u32, quality: u32, start: u32, step: usize) -> Vec<(u32, u32)> {
        let (width, height) = (self.image.width(), self.image.height());
        let mut points = vec![];
        for i in (start..width - w).step_by(step) {
            for j in (start..height - h).step_by(step) {
                let mut blank = true;
                // Rectangle:
                //
                // 		i,j			i+sizeX,j
                //
                // 		i,j+sizeY	i+sizeX,j+sizeY
                //
                for x in (i..i + w).step_by(quality as usize) {
                    for y in (j..j + h).step_by(quality as usize) {
                        let c = self.image.get_pixel(x, y);
                        if c.data[3] != 0 || c.data[0] != 0 && c.data[1] != 0 && c.data[2] != 0 {
                            blank = false;
                            break;
                        }
                    }
                    if !blank { break; }
                }
                if !blank {
                    continue;
                }
                points.push((i, j));
            }
        }
        points
    }

    fn find_blank(&self, w: u32, h: u32, quality: u32) -> (i32, i32) {
        for i in 0..30 {
            let points = self.find_blank_with_skip(w, h, quality, i, 30);
            if points.len() > 0 {
                let r = rand::thread_rng().gen_range::<usize>(0, points.len());
                let p = points[r];
                return (p.0 as i32, p.1 as i32);
            }
        }
        (-1, -1)
    }
}
